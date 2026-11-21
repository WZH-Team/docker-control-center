package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dcc/internal/config"
)

func TestLevelFilteringAndImportant(t *testing.T) {
	dir := t.TempDir()
	cfg := config.LoggingConfig{
		Enabled:    true,
		Level:      "warn",
		File:       "test.log",
		MaxSizeMB:  1,
		MaxBackups: 1,
	}

	m, err := New(cfg, dir, io.Discard)
	if err != nil {
		t.Fatalf("创建日志管理器失败: %v", err)
	}
	m.Logger().Info("info-should-drop")
	m.Logger().Warn("warn-should-keep")
	m.Important("important-should-keep")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test.log"))
	if err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "info-should-drop") {
		t.Fatal("warn 级别不应记录 info 日志")
	}
	if !strings.Contains(s, "warn-should-keep") {
		t.Fatal("warn 日志应被记录")
	}
	if !strings.Contains(s, "important-should-keep") {
		t.Fatal("Important 记录不受级别过滤影响")
	}
}

func TestLevelHotReload(t *testing.T) {
	dir := t.TempDir()
	m, err := New(config.LoggingConfig{Enabled: true, Level: "error", File: "test.log", MaxSizeMB: 1, MaxBackups: 1}, dir, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	m.Logger().Warn("warn-before-reload")

	// 热重载放宽级别后应记录 warn。
	if err := m.Apply(config.LoggingConfig{Enabled: true, Level: "info", File: "test.log", MaxSizeMB: 1, MaxBackups: 1}, dir); err != nil {
		t.Fatal(err)
	}
	m.Logger().Warn("warn-after-reload")
	_ = m.Close()

	data, err := os.ReadFile(filepath.Join(dir, "test.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "warn-before-reload") {
		t.Fatal("error 级别不应记录 warn 日志")
	}
	if !strings.Contains(s, "warn-after-reload") {
		t.Fatal("热重载后 warn 日志应被记录")
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug, "info": slog.LevelInfo, "INFO": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn,
		"error": slog.LevelError, "": slog.LevelWarn, "unknown": slog.LevelWarn,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Fatalf("ParseLevel(%q) = %v, 期望 %v", in, got, want)
		}
	}
}
