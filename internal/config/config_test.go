package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	return path
}

func TestGeneratePassword(t *testing.T) {
	for i := 0; i < 50; i++ {
		pw, err := GeneratePassword(16)
		if err != nil {
			t.Fatalf("GeneratePassword 失败: %v", err)
		}
		if len(pw) != 16 {
			t.Fatalf("密码长度应为 16，实际 %d", len(pw))
		}
		var hasLower, hasUpper, hasDigit, hasSpecial bool
		for _, r := range pw {
			switch {
			case strings.ContainsRune(lowerChars, r):
				hasLower = true
			case strings.ContainsRune(upperChars, r):
				hasUpper = true
			case strings.ContainsRune(digitChars, r):
				hasDigit = true
			case strings.ContainsRune(specialChars, r):
				hasSpecial = true
			default:
				t.Fatalf("密码包含非法字符 %q", r)
			}
		}
		if !hasLower || !hasUpper || !hasDigit || !hasSpecial {
			t.Fatalf("密码未覆盖全部字符类别: %q", pw)
		}
	}

	if _, err := GeneratePassword(4); err == nil {
		t.Fatal("过短密码应当报错")
	}
}

func TestGenerateAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, password, err := Generate(path)
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if len(password) != 16 {
		t.Fatalf("生成密码长度应为 16，实际 %d", len(password))
	}
	if cfg.Auth.Password != password {
		t.Fatal("返回的配置密码与明文不一致")
	}

	// 再次生成应当拒绝覆盖。
	if _, _, err := Generate(path); err == nil {
		t.Fatal("已存在的配置文件不应被覆盖")
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if loaded.Auth.Password != password {
		t.Fatal("加载后的密码与生成值不一致")
	}

	// 密码固定以双引号写入（与文档示例一致）。
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `password: "`) {
		t.Fatal("auth.password 应以双引号写入配置文件")
	}
	if loaded.Server.Port != DefaultPort {
		t.Fatalf("默认端口应为 %d，实际 %d", DefaultPort, loaded.Server.Port)
	}
	if loaded.Logging.Level != DefaultLogLevel {
		t.Fatalf("日志级别默认应为 %s，实际 %s", DefaultLogLevel, loaded.Logging.Level)
	}
	if !loaded.Logging.Enabled {
		t.Fatal("日志应默认开启")
	}
	if loaded.SSL.Enabled {
		t.Fatal("SSL 应默认关闭")
	}
	if loaded.LogFilePath() != filepath.Join(dir, "logs", "dcc.log") {
		t.Fatalf("日志路径解析错误: %s", loaded.LogFilePath())
	}
}

func TestLoadDefaultsAndMigration(t *testing.T) {
	// 极简旧配置：无 version、无多个字段。
	path := writeTemp(t, "config.yaml", "auth:\n  username: root\n  password: oldpassword123\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("缺失 version 应迁移为 %d，实际 %d", CurrentVersion, cfg.Version)
	}
	if cfg.Auth.Username != "root" {
		t.Fatalf("用户名应保留为 root，实际 %s", cfg.Auth.Username)
	}
	if cfg.Server.Port != DefaultPort {
		t.Fatalf("缺失端口应补默认值，实际 %d", cfg.Server.Port)
	}
	if !cfg.Logging.Enabled || cfg.Logging.MaxBackups != 3 {
		t.Fatal("日志默认值补全失败")
	}
}

func TestLoadIgnoresUnknownFields(t *testing.T) {
	path := writeTemp(t, "config.yaml", `
version: 1
future_field: hello
server:
  port: 9000
  future_flag: true
auth:
  username: admin
  password: password123
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("未知字段不应导致加载失败: %v", err)
	}
	if cfg.Server.Port != 9000 {
		t.Fatalf("端口应为 9000，实际 %d", cfg.Server.Port)
	}
}

func TestLoadValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"端口越界", "server:\n  port: 70000\nauth:\n  username: a\n  password: password123\n"},
		{"密码过短", "auth:\n  username: a\n  password: short\n"},
		{"密码含换行", "auth:\n  username: a\n  password: \"pass\\nword123\"\n"},
		{"日志级别非法", "logging:\n  level: verbose\nauth:\n  username: a\n  password: password123\n"},
		{"SSL 缺证书", "ssl:\n  enabled: true\nauth:\n  username: a\n  password: password123\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "config.yaml", tc.content)
			if _, err := Load(path); err == nil {
				t.Fatal("非法配置应当报错")
			}
		})
	}
}

func TestEmptyUsernameDefaultsToAdmin(t *testing.T) {
	path := writeTemp(t, "config.yaml", "auth:\n  username: \"\"\n  password: password123\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("空用户名应补默认值而非报错: %v", err)
	}
	if cfg.Auth.Username != "admin" {
		t.Fatalf("空用户名应默认为 admin，实际 %q", cfg.Auth.Username)
	}
}

func TestWriteBackPreservesComments(t *testing.T) {
	path := writeTemp(t, "config.yaml", generatedHeader+`
version: 1
server:
  port: 8080 # 监听端口
auth:
  username: admin
  password: password123
compose_projects: []
`)

	projects := []ComposeProject{{Name: "demo", Path: "/opt/demo"}, {Path: "/opt/other"}}
	if err := SetComposeProjects(path, projects); err != nil {
		t.Fatalf("SetComposeProjects 失败: %v", err)
	}
	if err := SetAuth(path, "root", "newpassword456"); err != nil {
		t.Fatalf("SetAuth 失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# 监听端口") {
		t.Fatalf("注释未被保留:\n%s", text)
	}
	if !strings.Contains(text, `password: "newpassword456"`) {
		t.Fatal("写回的 auth.password 应以双引号包裹")
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("写回后加载失败: %v", err)
	}
	if cfg.Auth.Username != "root" || cfg.Auth.Password != "newpassword456" {
		t.Fatalf("认证信息写回错误: %+v", cfg.Auth)
	}
	if len(cfg.ComposeProjects) != 2 {
		t.Fatalf("compose_projects 写回错误: %+v", cfg.ComposeProjects)
	}
	if cfg.ComposeProjects[0].Name != "demo" || cfg.ComposeProjects[1].Name != "" {
		t.Fatalf("compose_projects 字段错误: %+v", cfg.ComposeProjects)
	}
}

func TestWatcherHotReload(t *testing.T) {
	path := writeTemp(t, "config.yaml", "auth:\n  username: admin\n  password: password123\n")

	loaded := make(chan *Config, 4)
	failed := make(chan error, 4)
	w := NewWatcher(path, nil, func(c *Config) { loaded <- c }, func(e error) { failed <- e })
	w.interval = 50 * time.Millisecond
	w.snapshot()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// 修改配置，等待热重载回调。
	newContent := "server:\n  port: 9999\nauth:\n  username: admin\n  password: password123\n"
	if err := os.WriteFile(path, []byte(newContent), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-loaded:
		if cfg.Server.Port != 9999 {
			t.Fatalf("热重载后端口应为 9999，实际 %d", cfg.Server.Port)
		}
	case err := <-failed:
		t.Fatalf("热重载失败: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("等待热重载超时")
	}

	// 写入非法配置不应触发重载回调。
	if err := os.WriteFile(path, []byte("server:\n  port: -1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case cfg := <-loaded:
		t.Fatalf("非法配置不应被载入: %+v", cfg)
	case err := <-failed:
		if err == nil {
			t.Fatal("非法配置应上报错误")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("等待错误回调超时")
	}
}

func TestResolveAndComposeLookup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "auth:\n  username: admin\n  password: password123\ncompose_projects:\n  - name: demo\n    path: ./apps/demo\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	abs := filepath.Join(dir, "apps", "demo")
	if got := cfg.ComposeProjectPath(cfg.ComposeProjects[0]); got != abs {
		t.Fatalf("项目路径解析错误: %s != %s", got, abs)
	}
	if _, ok := cfg.FindComposeProject(abs); !ok {
		t.Fatal("按绝对路径查找项目失败")
	}
	if _, ok := cfg.FindComposeProject("demo"); !ok {
		t.Fatal("按名称查找项目失败")
	}
	if _, ok := cfg.FindComposeProject("missing"); ok {
		t.Fatal("不应匹配到不存在的项目")
	}

	// SSL 证书解析：cert_dir 相对路径。
	cfg.SSL.CertDir = "certs"
	cert, key, err := cfg.SSLCertFiles()
	if err != nil {
		t.Fatal(err)
	}
	if cert != filepath.Join(dir, "certs", "cert.pem") || key != filepath.Join(dir, "certs", "key.pem") {
		t.Fatalf("证书路径解析错误: %s / %s", cert, key)
	}
}
