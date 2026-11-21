package config

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Watcher 以轮询方式监听配置文件变化并触发热重载。
// 使用轮询而非文件系统事件监听：跨平台行为一致（Windows/Linux），
// 且能正确处理编辑器“原子替换式保存”等场景。
type Watcher struct {
	path     string
	interval time.Duration
	onReload func(*Config)
	onError  func(error)
	logger   *slog.Logger

	mu       sync.Mutex
	lastMod  time.Time
	lastSize int64
	lastSum  string
}

// NewWatcher 创建配置热重载监听器。
// 基线文件状态在构造时即记录，避免与调用方随后的写入产生竞态。
func NewWatcher(path string, logger *slog.Logger, onReload func(*Config), onError func(error)) *Watcher {
	w := &Watcher{
		path:     path,
		interval: 2 * time.Second,
		onReload: onReload,
		onError:  onError,
		logger:   logger,
	}
	w.snapshot()
	return w
}

// Run 阻塞运行监听循环，直到 ctx 取消。
// 注意：此处不重复记录基线，否则可能把调用方的并发写入误记为基线，导致漏检。
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// snapshot 记录当前文件状态。
func (w *Watcher) snapshot() {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, err := os.Stat(w.path)
	if err != nil {
		w.lastMod, w.lastSize, w.lastSum = time.Time{}, -1, ""
		return
	}
	w.lastMod, w.lastSize = st.ModTime(), st.Size()
}

// check 检测文件是否变化，变化则重新加载并回调。
func (w *Watcher) check() {
	st, err := os.Stat(w.path)
	if err != nil {
		// 文件被删除等场景：保持现有配置不动，仅记录一次状态变化。
		w.mu.Lock()
		missing := w.lastSize != -1
		w.lastMod, w.lastSize = time.Time{}, -1
		w.mu.Unlock()
		if missing && w.logger != nil {
			w.logger.Warn("配置文件暂时不可读，保持当前配置", "path", w.path, "error", err)
		}
		return
	}

	w.mu.Lock()
	changed := !st.ModTime().Equal(w.lastMod) || st.Size() != w.lastSize
	if changed {
		w.lastMod, w.lastSize = st.ModTime(), st.Size()
	}
	w.mu.Unlock()
	if !changed {
		return
	}

	// 文件可能仍在写入中，稍等后重读，避免读到半截内容。
	time.Sleep(150 * time.Millisecond)
	cfg, err := Load(w.path)
	if err != nil {
		if w.onError != nil {
			w.onError(err)
		}
		return
	}
	if w.logger != nil {
		w.logger.Info("检测到配置变更，已自动热重载", "path", w.path)
	}
	if w.onReload != nil {
		w.onReload(cfg)
	}
}

// ErrNoReload 保留：未来用于同步等待一次重载完成。
var ErrNoReload = errors.New("no reload")
