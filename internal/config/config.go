// Package config 负责 DCC 配置文件 (config.yaml) 的加载、生成、校验、写回与热重载。
//
// 向后兼容策略：
//   - 配置带有 version 字段，旧版本（或缺失）配置会被就地迁移为当前版本；
//   - 未知字段被忽略并保留（不使用严格解码），便于新版程序平滑迁移旧配置；
//   - 相对路径统一相对于配置文件所在目录解析。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// CurrentVersion 当前配置结构版本。
	CurrentVersion = 1
	// DefaultPort 默认监听端口。
	DefaultPort = 8080
	// MinPasswordLen 登录密码最小长度。
	MinPasswordLen = 8
	// DefaultLogFile 默认日志文件（相对配置文件目录）。
	DefaultLogFile = "logs/dcc.log"
	// DefaultLogLevel 默认日志级别。
	DefaultLogLevel = "warn"
)

// LogLevels 支持的日志级别。
var LogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

// ServerConfig Web 服务监听配置。
type ServerConfig struct {
	// Host 监听地址，留空监听所有网卡。
	Host string `yaml:"host"`
	// Port 监听端口。
	Port int `yaml:"port"`
}

// AuthConfig 登录鉴权配置。
type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LoggingConfig 日志配置。日志默认开启，记录报错、警告以及首次生成的登录密码。
type LoggingConfig struct {
	// Enabled 日志开关（默认开启）。
	Enabled bool `yaml:"enabled"`
	// Level 日志存储级别：debug / info / warn / error（默认 warn）。
	// info 级别可记录操作日志；必须落盘的记录（如首次生成的密码）不受此过滤影响。
	Level string `yaml:"level"`
	// File 日志文件路径，相对路径基于配置文件所在目录。
	File string `yaml:"file"`
	// MaxSizeMB 单个日志文件大小上限（MB），超出后轮转。
	MaxSizeMB int `yaml:"max_size_mb"`
	// MaxBackups 轮转保留的备份文件数量。
	MaxBackups int `yaml:"max_backups"`
}

// SSLConfig HTTPS/SSL 配置，默认关闭。
type SSLConfig struct {
	// Enabled 是否启用 HTTPS。
	Enabled bool `yaml:"enabled"`
	// CertDir 证书目录（绝对路径或相对路径），目录内需包含 cert.pem 与 key.pem。
	CertDir string `yaml:"cert_dir"`
	// CertFile 证书文件路径（优先于 cert_dir）。
	CertFile string `yaml:"cert_file"`
	// KeyFile 私钥文件路径（优先于 cert_dir）。
	KeyFile string `yaml:"key_file"`
	// KeepHTTP 开启 HTTPS 后是否保留 HTTP 服务。
	KeepHTTP bool `yaml:"keep_http"`
}

// DockerConfig Docker 连接配置。
type DockerConfig struct {
	// Host Docker 守护进程地址，留空使用默认 unix:///var/run/docker.sock。
	// Windows 开发环境可填写 tcp://127.0.0.1:2375。
	Host string `yaml:"host"`
}

// ComposeProject 已导入的 Docker Compose 项目目录。
type ComposeProject struct {
	// Name 项目名称，留空时按目录名推导。
	Name string `yaml:"name,omitempty"`
	// Path 项目目录（绝对路径或相对配置文件目录的相对路径）。
	Path string `yaml:"path"`
}

// Config DCC 总配置。
type Config struct {
	Version         int              `yaml:"version"`
	Server          ServerConfig     `yaml:"server"`
	Auth            AuthConfig       `yaml:"auth"`
	Logging         LoggingConfig    `yaml:"logging"`
	SSL             SSLConfig        `yaml:"ssl"`
	Docker          DockerConfig     `yaml:"docker"`
	ComposeProjects []ComposeProject `yaml:"compose_projects"`

	// Path 配置文件完整路径（运行时填充）。
	Path string `yaml:"-"`
	// BaseDir 配置文件所在目录，相对路径基于该目录解析（运行时填充）。
	BaseDir string `yaml:"-"`
}

// NewDefault 返回带默认值的配置。
func NewDefault() *Config {
	return &Config{
		Version: CurrentVersion,
		Server: ServerConfig{
			Host: "",
			Port: DefaultPort,
		},
		Auth: AuthConfig{
			Username: "admin",
		},
		Logging: LoggingConfig{
			Enabled:    true,
			Level:      DefaultLogLevel,
			File:       DefaultLogFile,
			MaxSizeMB:  5,
			MaxBackups: 3,
		},
		SSL: SSLConfig{
			Enabled:  false,
			KeepHTTP: false,
		},
		ComposeProjects: []ComposeProject{},
	}
}

// Load 读取并解析配置文件。缺失字段自动补默认值，旧版本配置自动迁移。
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("无法解析配置文件路径 %q: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := NewDefault()
	// 非严格解码：未知字段被忽略，保证新旧版本配置双向兼容。
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %q 失败: %w", abs, err)
	}

	cfg.Path = abs
	cfg.BaseDir = filepath.Dir(abs)
	cfg.migrate()
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.ResolvePaths()
	return cfg, nil
}

// migrate 将旧版本配置就地升级为当前版本。
func (c *Config) migrate() {
	if c.Version <= 0 {
		// v0/未标注版本：视为 v1 结构（当前结构自 v1 起），仅补齐版本号。
		c.Version = CurrentVersion
	}
	// 未来版本的迁移分支在此追加，例如：
	// if c.Version < 2 { ...; c.Version = 2 }
}

// applyDefaults 对缺失字段补全默认值。
func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = DefaultPort
	}
	if strings.TrimSpace(c.Auth.Username) == "" {
		c.Auth.Username = "admin"
	}
	if c.Logging.File == "" {
		c.Logging.File = DefaultLogFile
	}
	if c.Logging.Level == "" {
		c.Logging.Level = DefaultLogLevel
	}
	c.Logging.Level = strings.ToLower(strings.TrimSpace(c.Logging.Level))
	if c.Logging.MaxSizeMB <= 0 {
		c.Logging.MaxSizeMB = 5
	}
	if c.Logging.MaxBackups <= 0 {
		c.Logging.MaxBackups = 3
	}
	if c.ComposeProjects == nil {
		c.ComposeProjects = []ComposeProject{}
	}
}

// Validate 校验配置合法性。
func (c *Config) Validate() error {
	if c.Version > CurrentVersion {
		// 高版本配置：向前兼容，仅提示，不阻断启动。
		fmt.Fprintf(os.Stderr, "[DCC] 警告: 配置文件版本 %d 高于程序支持的 %d，未知字段将被忽略\n", c.Version, CurrentVersion)
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("配置错误: server.port 必须在 1-65535 之间，当前为 %d", c.Server.Port)
	}
	if strings.TrimSpace(c.Auth.Username) == "" {
		return errors.New("配置错误: auth.username 不能为空")
	}
	if len(c.Auth.Password) < MinPasswordLen {
		return fmt.Errorf("配置错误: auth.password 长度不得少于 %d 个字符", MinPasswordLen)
	}
	if !LogLevels[c.Logging.Level] {
		return fmt.Errorf("配置错误: logging.level 必须为 debug / info / warn / error 之一，当前为 %q", c.Logging.Level)
	}
	if strings.ContainsAny(c.Auth.Username, "\r\n") || strings.ContainsAny(c.Auth.Password, "\r\n") {
		return errors.New("配置错误: auth.username / auth.password 不能包含换行符")
	}
	if c.SSL.Enabled {
		cert, key, err := c.SSLCertFiles()
		if err != nil {
			return err
		}
		for _, f := range []string{cert, key} {
			if st, err := os.Stat(f); err != nil || st.IsDir() {
				return fmt.Errorf("配置错误: SSL 证书文件不存在或不可读: %s", f)
			}
		}
	}
	for i, p := range c.ComposeProjects {
		if strings.TrimSpace(p.Path) == "" {
			return fmt.Errorf("配置错误: compose_projects[%d].path 不能为空", i)
		}
	}
	return nil
}

// ResolvePaths 规范化内部路径字段（保留用户书写的相对形式，仅做清洗）。
func (c *Config) ResolvePaths() {
	c.Logging.File = filepath.ToSlash(strings.TrimSpace(c.Logging.File))
	for i := range c.ComposeProjects {
		c.ComposeProjects[i].Path = filepath.Clean(c.ComposeProjects[i].Path)
	}
}

// Resolve 将路径解析为绝对路径：绝对路径原样返回，相对路径基于配置文件目录。
func (c *Config) Resolve(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(c.BaseDir, path))
}

// LogFilePath 返回日志文件绝对路径。
func (c *Config) LogFilePath() string {
	return c.Resolve(c.Logging.File)
}

// SSLCertFiles 返回生效的证书与私钥绝对路径。
// 优先使用 cert_file/key_file；否则使用 cert_dir 下的 cert.pem 与 key.pem。
func (c *Config) SSLCertFiles() (cert, key string, err error) {
	certFile := strings.TrimSpace(c.SSL.CertFile)
	keyFile := strings.TrimSpace(c.SSL.KeyFile)
	if certFile != "" && keyFile != "" {
		return c.Resolve(certFile), c.Resolve(keyFile), nil
	}
	dir := strings.TrimSpace(c.SSL.CertDir)
	if dir == "" {
		return "", "", errors.New("配置错误: ssl.enabled 为 true 时必须配置 ssl.cert_dir 或 ssl.cert_file/key_file")
	}
	abs := c.Resolve(dir)
	cert = filepath.Join(abs, "cert.pem")
	key = filepath.Join(abs, "key.pem")
	return cert, key, nil
}

// ComposeProjectPath 返回 Compose 项目目录的绝对路径。
func (c *Config) ComposeProjectPath(p ComposeProject) string {
	return c.Resolve(p.Path)
}

// FindComposeProject 按路径或名称查找项目。
func (c *Config) FindComposeProject(key string) (ComposeProject, bool) {
	target := filepath.Clean(c.Resolve(key))
	for _, p := range c.ComposeProjects {
		if filepath.Clean(c.ComposeProjectPath(p)) == target || p.Name == key {
			return p, true
		}
	}
	return ComposeProject{}, false
}

// Clone 返回配置的拷贝（用于并发安全的读取快照）。
func (c *Config) Clone() *Config {
	cp := *c
	cp.ComposeProjects = append([]ComposeProject(nil), c.ComposeProjects...)
	return &cp
}

// ConfigModTime 返回配置文件修改时间（用于热重载检测）。
func ConfigModTime(path string) (time.Time, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return st.ModTime(), true
}
