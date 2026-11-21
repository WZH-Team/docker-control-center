// Package logging 提供 DCC 的结构化日志能力：
// 基于 log/slog，默认写入日志文件并按大小轮转，同时支持运行时开关与热更新。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dcc/internal/config"
)

// RotatingWriter 按大小轮转的文件写入器。非并发安全由内部互斥锁保证。
type RotatingWriter struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	size       int64
	file       *os.File
}

// NewRotatingWriter 打开（或创建）日志文件并返回轮转写入器。
func NewRotatingWriter(path string, maxSizeMB, maxBackups int) (*RotatingWriter, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 5
	}
	if maxBackups <= 0 {
		maxBackups = 3
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	w := &RotatingWriter{
		path:       path,
		maxSize:    int64(maxSizeMB) * 1024 * 1024,
		maxBackups: maxBackups,
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("读取日志文件状态失败: %w", err)
	}
	w.file = f
	w.size = st.Size()
	return nil
}

// Write 实现 io.Writer，超出大小上限时先轮转。
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return len(p), nil
	}
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate 执行轮转：dcc.log -> dcc.log.1 -> dcc.log.2 ...
func (w *RotatingWriter) rotate() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	for i := w.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
	_ = os.Rename(w.path, w.path+".1")
	return w.open()
}

// Close 关闭底层文件。
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// swapWriter 允许在不停机的情况下切换日志输出目标（用于配置热重载）。
type swapWriter struct {
	mu  sync.Mutex
	dst io.Writer
}

func (s *swapWriter) set(w io.Writer) {
	s.mu.Lock()
	s.dst = w
	s.mu.Unlock()
}

func (s *swapWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	dst := s.dst
	s.mu.Unlock()
	if dst == nil {
		return len(p), nil
	}
	return dst.Write(p)
}

// Manager 管理全局日志器，支持热更新日志配置。
type Manager struct {
	sw     *swapWriter
	logger *slog.Logger
	level  *slog.LevelVar
	// important 记录必须落盘的信息（如首次生成的密码），不受 level 过滤影响。
	important *slog.Logger

	mu     sync.Mutex
	writer *RotatingWriter
	// console 为程序启动/停止等关键信息保留的独立控制台输出。
	console io.Writer
}

// ParseLevel 解析日志级别（debug / info / warn|warning / error），无法识别时回退 warn。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

// New 创建日志管理器。console 通常为 os.Stdout。
func New(cfg config.LoggingConfig, baseDir string, console io.Writer) (*Manager, error) {
	m := &Manager{
		sw:      &swapWriter{dst: io.Discard},
		level:   new(slog.LevelVar),
		console: console,
	}
	m.level.Set(ParseLevel(cfg.Level))
	m.logger = slog.New(slog.NewTextHandler(m.sw, &slog.HandlerOptions{Level: m.level}))
	m.important = slog.New(slog.NewTextHandler(m.sw, &slog.HandlerOptions{Level: slog.LevelDebug}))
	// 即使日志文件不可写也返回可用的管理器（回退到控制台输出）。
	if err := m.Apply(cfg, baseDir); err != nil {
		return m, err
	}
	return m, nil
}

// Apply 应用新的日志配置（热重载时调用）。
func (m *Manager) Apply(cfg config.LoggingConfig, baseDir string) error {
	m.level.Set(ParseLevel(cfg.Level))

	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭旧文件。
	if m.writer != nil {
		_ = m.writer.Close()
		m.writer = nil
	}

	if !cfg.Enabled {
		m.sw.set(io.Discard)
		return nil
	}

	file := cfg.File
	if file == "" {
		file = config.DefaultLogFile
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(baseDir, file)
	}
	w, err := NewRotatingWriter(file, cfg.MaxSizeMB, cfg.MaxBackups)
	if err != nil {
		// 日志不可写不应阻断服务：回退到仅控制台。
		m.sw.set(m.console)
		return err
	}
	m.writer = w
	// 文件 + 控制台双写。
	m.sw.set(io.MultiWriter(w, m.console))
	return nil
}

// Logger 返回全局 slog 实例。
func (m *Manager) Logger() *slog.Logger {
	return m.logger
}

// Important 记录必须落盘的信息（如首次生成的密码），不受日志级别过滤影响。
func (m *Manager) Important(msg string, args ...any) {
	m.important.Info(msg, args...)
}

// LogFilePath 当前日志文件路径（未启用日志时为空）。
func (m *Manager) LogFilePath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writer == nil {
		return ""
	}
	return m.writer.path
}

// Close 释放资源。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writer != nil {
		err := m.writer.Close()
		m.writer = nil
		return err
	}
	return nil
}

// StartupMessage 输出到控制台的启动信息（不受日志开关影响）。
func (m *Manager) StartupMessage(format string, args ...any) {
	if m.console == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	_, _ = io.WriteString(m.console, msg)
}
