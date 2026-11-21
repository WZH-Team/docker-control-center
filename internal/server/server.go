package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"dcc/internal/config"
	"dcc/internal/docker"
	"dcc/internal/jobs"
)

// State 运行期配置状态（支持热重载的原子化读取）。
type State struct {
	cfg atomic.Pointer[config.Config]
}

// NewState 创建状态容器。
func NewState(cfg *config.Config) *State {
	s := &State{}
	s.Set(cfg)
	return s
}

// Get 返回当前配置快照。
func (s *State) Get() *config.Config { return s.cfg.Load() }

// Set 原子更新配置。
func (s *State) Set(c *config.Config) { s.cfg.Store(c) }

// Deps 服务依赖。
type Deps struct {
	State        *State
	Logger       *slog.Logger
	Jobs         *jobs.Manager
	Docker       func() *docker.Client
	Compose      func() *docker.ComposeRunner
	ReloadConfig func() error
	Version      string
	Started      time.Time
}

// Service Web 服务：路由、鉴权与监听管理（支持配置热重载）。
type Service struct {
	deps     *Deps
	handler  http.Handler
	sessions *SessionManager
	limiter  *loginLimiter

	mu         sync.Mutex
	httpSrv    *http.Server
	listener   net.Listener
	listenDesc string
	certFP     string
}

// New 创建 Web 服务（不开始监听）。
func New(deps *Deps) *Service {
	deps.Logger = ensureLogger(deps.Logger)
	s := &Service{
		deps:     deps,
		sessions: NewSessionManager(),
		limiter:  newLoginLimiter(),
	}
	s.handler = securityHeaders(s.withRecover(s.withRequestLog(s.routes())))
	return s
}

// Handler 返回根 HTTP 处理器（测试用）。
func (s *Service) Handler() http.Handler { return s.handler }

// Start 按当前配置启动监听。
func (s *Service) Start() error {
	cfg := s.deps.State.Get()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked(cfg)
}

// ApplyConfig 热应用新配置：监听参数或证书内容变化时重建监听器。
func (s *Service) ApplyConfig(oldCfg, newCfg *config.Config) error {
	if !listenConfigChanged(oldCfg, newCfg) && certFingerprint(newCfg) == s.certFP {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopLocked()
	if err := s.startLocked(newCfg); err != nil {
		// 回滚到旧配置。
		if rerr := s.startLocked(oldCfg); rerr != nil {
			return fmt.Errorf("切换监听失败: %v（回滚亦失败: %v）", err, rerr)
		}
		return fmt.Errorf("切换监听失败，已回滚: %w", err)
	}
	return nil
}

// certFingerprint 计算证书/私钥内容指纹（用于证书续期免重启检测）。
func certFingerprint(cfg *config.Config) string {
	if !cfg.SSL.Enabled {
		return ""
	}
	certFile, keyFile, err := cfg.SSLCertFiles()
	if err != nil {
		return ""
	}
	h := sha256.New()
	for _, f := range []string{certFile, keyFile} {
		if data, err := os.ReadFile(f); err == nil {
			_, _ = h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// listenConfigChanged 判断监听相关配置是否变化。
func listenConfigChanged(a, b *config.Config) bool {
	return a.Server.Host != b.Server.Host ||
		a.Server.Port != b.Server.Port ||
		a.SSL.Enabled != b.SSL.Enabled ||
		a.SSL.KeepHTTP != b.SSL.KeepHTTP ||
		a.SSL.CertDir != b.SSL.CertDir ||
		a.SSL.CertFile != b.SSL.CertFile ||
		a.SSL.KeyFile != b.SSL.KeyFile
}

// startLocked 启动监听（调用方持有锁）。
func (s *Service) startLocked(cfg *config.Config) error {
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	if cfg.SSL.Enabled {
		tlsCfg, err := buildTLSConfig(cfg)
		if err != nil {
			_ = ln.Close()
			return err
		}
		if cfg.SSL.KeepHTTP {
			// 同端口同时接受 HTTPS 与 HTTP（按 TLS 握手特征自动识别）。
			ln = &sniffListener{Listener: ln, tlsCfg: tlsCfg, allowPlain: true}
			s.listenDesc = fmt.Sprintf("https://%s (保留 http)", addr)
		} else {
			ln = tls.NewListener(ln, tlsCfg)
			s.listenDesc = fmt.Sprintf("https://%s", addr)
		}
	} else {
		s.listenDesc = fmt.Sprintf("http://%s", addr)
	}

	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(s.deps.Logger.Handler(), slog.LevelWarn),
	}
	s.httpSrv = srv
	s.listener = ln
	s.certFP = certFingerprint(cfg)

	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.deps.Logger.Error("Web 服务异常退出", "error", err)
		}
	}()

	s.deps.Logger.Info("Web 服务已启动", "listen", s.listenDesc)
	return nil
}

// stopLocked 停止监听（调用方持有锁）。
func (s *Service) stopLocked() {
	if s.httpSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)
	s.httpSrv = nil
	s.listener = nil
}

// ListenDesc 当前监听描述。
func (s *Service) ListenDesc() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listenDesc
}

// Shutdown 优雅关闭。
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpSrv == nil {
		return nil
	}
	err := s.httpSrv.Shutdown(ctx)
	s.httpSrv = nil
	s.listener = nil
	return err
}

// buildTLSConfig 构建 TLS 配置（每次热重载重新读取证书，支持证书续期免重启）。
func buildTLSConfig(cfg *config.Config) (*tls.Config, error) {
	certFile, keyFile, err := cfg.SSLCertFiles()
	if err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载 SSL 证书失败（%s / %s）: %w", certFile, keyFile, err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1"},
	}, nil
}

// sniffListener 同端口双协议监听：根据首字节识别 TLS 握手，否则按明文 HTTP 处理。
type sniffListener struct {
	net.Listener
	tlsCfg     *tls.Config
	allowPlain bool
}

// Accept 接受连接并按协议分发。
func (l *sniffListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		var head [1]byte
		n, err := conn.Read(head[:])
		_ = conn.SetReadDeadline(time.Time{})
		if err != nil {
			_ = conn.Close()
			continue
		}
		replay := &bufferedConn{Conn: conn, prefix: head[:n]}
		// TLS 记录层握手类型固定为 0x16。
		if head[0] == 0x16 {
			return tls.Server(replay, l.tlsCfg), nil
		}
		if l.allowPlain {
			return replay, nil
		}
		_ = conn.Close()
	}
}

// bufferedConn 支持回放预读字节的连接。
type bufferedConn struct {
	net.Conn
	prefix []byte
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	if len(b.prefix) > 0 {
		n := copy(p, b.prefix)
		b.prefix = b.prefix[n:]
		return n, nil
	}
	return b.Conn.Read(p)
}

var _ io.Reader = (*bufferedConn)(nil)
