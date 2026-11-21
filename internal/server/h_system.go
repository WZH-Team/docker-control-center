package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"dcc/internal/config"
	"dcc/internal/docker"
)

// ---------- 会话与鉴权 ----------

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionInfo struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	CSRFToken     string `json:"csrfToken,omitempty"`
}

// handleSession 返回当前登录状态。
func (s *Service) handleSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, sessionInfo{Authenticated: false})
		return
	}
	writeJSON(w, http.StatusOK, sessionInfo{
		Authenticated: true,
		Username:      sess.Username,
		CSRFToken:     sess.CSRF,
	})
}

// handleLogin 登录。
func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	ip := clientIP(r)

	if !s.limiter.Allow(ip, req.Username) {
		s.deps.Logger.Warn("登录尝试被限流", "ip", ip, "user", req.Username)
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "登录尝试过于频繁，请稍后再试")
		return
	}

	cfg := s.deps.State.Get()
	if !verifyCredentials(cfg, req.Username, req.Password) {
		s.limiter.Fail(ip, req.Username)
		s.deps.Logger.Warn("登录失败", "ip", ip, "user", req.Username)
		writeErr(w, http.StatusUnauthorized, CodeLoginFailed, "用户名或密码错误")
		return
	}

	s.limiter.Reset(ip, req.Username)
	sess := s.sessions.Create(req.Username)
	setSessionCookie(w, r, cfg, sess.ID)
	s.deps.Logger.Info("登录成功", "ip", ip, "user", req.Username)
	writeJSON(w, http.StatusOK, sessionInfo{
		Authenticated: true,
		Username:      sess.Username,
		CSRFToken:     sess.CSRF,
	})
}

// handleLogout 退出登录。
func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request, sess *Session) {
	s.sessions.Delete(sess.ID)
	clearSessionCookie(w, r, s.deps.State.Get())
	s.deps.Logger.Info("退出登录", "user", sess.Username, "ip", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- 运行状态 ----------

type appStatus struct {
	Version    string `json:"version"`
	StartedAt  string `json:"startedAt"`
	Listen     string `json:"listen"`
	Platform   string `json:"platform"`
	ConfigPath string `json:"configPath"`
}

type dockerStatus struct {
	Reachable        bool                    `json:"reachable"`
	BinaryFound      bool                    `json:"binaryFound"`
	ComposeAvailable bool                    `json:"composeAvailable"`
	Host             string                  `json:"host"`
	Version          *docker.VersionResponse `json:"version,omitempty"`
	Error            string                  `json:"error,omitempty"`
}

// handleStatus 返回应用与 Docker 运行状态。
func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request, _ *Session) {
	cfg := s.deps.State.Get()

	status := dockerStatus{Host: cfg.Docker.Host}
	if status.Host == "" {
		status.Host = docker.DefaultHost()
	}
	if status.Host == "" {
		status.Host = "(未配置)"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		status.BinaryFound = true
	}

	client := s.deps.Docker()
	if client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if v, err := client.EnsureReady(ctx); err != nil {
			status.Error = err.Error()
		} else {
			status.Reachable = true
			status.Version = v
		}
	}

	if status.Reachable {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if runner := s.deps.Compose(); runner != nil {
			status.ComposeAvailable = runner.Available(ctx)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app": appStatus{
			Version:    s.deps.Version,
			StartedAt:  s.deps.Started.Format(time.RFC3339),
			Listen:     s.ListenDesc(),
			Platform:   runtime.GOOS + "/" + runtime.GOARCH,
			ConfigPath: cfg.Path,
		},
		"docker": status,
	})
}

// handleDockerInstall 触发 Docker 安装脚本（固定命令 `curl -fsSL https://get.docker.com | bash`）。
func (s *Service) handleDockerInstall(w http.ResponseWriter, r *http.Request, sess *Session) {
	if !docker.IsLinux() {
		writeErr(w, http.StatusBadRequest, CodeUnsupportedOS, "自动安装仅支持 Debian/Ubuntu 等 Linux 系统")
		return
	}

	// 已在运行则无须安装。
	client := s.deps.Docker()
	if client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx); err == nil {
			writeErr(w, http.StatusConflict, CodeConflict, "Docker 已在运行，无须安装")
			return
		}
	}

	s.deps.Logger.Info("开始安装 Docker", "user", sess.Username, "ip", clientIP(r))
	job := s.deps.Jobs.Start("docker.install", "安装 Docker", func(ctx context.Context, out io.Writer) error {
		if err := docker.InstallDocker(ctx, out); err != nil {
			return err
		}
		// 安装完成后重置 Compose 探测缓存。
		if runner := s.deps.Compose(); runner != nil {
			runner.ResetDetection()
		}
		return nil
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID})
}

// ---------- 日志与配置 ----------

// handleLogsTail 返回日志文件尾部内容。
func (s *Service) handleLogsTail(w http.ResponseWriter, r *http.Request, _ *Session) {
	cfg := s.deps.State.Get()
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 && n <= 1000 {
			lines = n
		}
	}

	path := cfg.LogFilePath()
	data := map[string]any{
		"enabled": cfg.Logging.Enabled,
		"path":    path,
		"lines":   []string{},
	}
	if !cfg.Logging.Enabled {
		writeJSON(w, http.StatusOK, data)
		return
	}

	content, err := tailFile(path, 256*1024)
	if err != nil {
		if !os.IsNotExist(err) {
			writeErr(w, http.StatusInternalServerError, CodeInternal, "读取日志失败: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, data)
		return
	}

	all := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	data["lines"] = all
	writeJSON(w, http.StatusOK, data)
}

// tailFile 读取文件尾部（最多 max 字节）。
func tailFile(path string, max int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	var start int64
	if size > max {
		start = size - max
	}
	if _, err := f.Seek(start, 0); err != nil {
		return "", err
	}
	buf := make([]byte, size-start)
	n, err := f.Read(buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

// handleConfigView 返回脱敏后的配置视图（不含密码）。
func (s *Service) handleConfigView(w http.ResponseWriter, r *http.Request, _ *Session) {
	cfg := s.deps.State.Get()
	projects := make([]map[string]string, 0, len(cfg.ComposeProjects))
	for _, p := range cfg.ComposeProjects {
		projects = append(projects, map[string]string{
			"name":    p.Name,
			"path":    p.Path,
			"absPath": cfg.ComposeProjectPath(p),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configPath": cfg.Path,
		"version":    cfg.Version,
		"server": map[string]any{
			"host": cfg.Server.Host,
			"port": cfg.Server.Port,
		},
		"auth": map[string]any{
			"username": cfg.Auth.Username,
		},
		"logging": map[string]any{
			"enabled":    cfg.Logging.Enabled,
			"level":      cfg.Logging.Level,
			"file":       cfg.Logging.File,
			"maxSizeMB":  cfg.Logging.MaxSizeMB,
			"maxBackups": cfg.Logging.MaxBackups,
		},
		"ssl": map[string]any{
			"enabled":  cfg.SSL.Enabled,
			"keepHTTP": cfg.SSL.KeepHTTP,
			"certDir":  cfg.SSL.CertDir,
			"certFile": cfg.SSL.CertFile,
			"keyFile":  cfg.SSL.KeyFile,
		},
		"docker": map[string]any{
			"host": cfg.Docker.Host,
		},
		"composeProjects": projects,
	})
}

// handleConfigReload 手动触发热重载。
func (s *Service) handleConfigReload(w http.ResponseWriter, r *http.Request, sess *Session) {
	if s.deps.ReloadConfig == nil {
		writeErr(w, http.StatusInternalServerError, CodeInternal, "配置重载不可用")
		return
	}
	if err := s.deps.ReloadConfig(); err != nil {
		s.deps.Logger.Error("配置重载失败", "user", sess.Username, "error", err)
		writeErr(w, http.StatusBadRequest, CodeConfigInvalid, err.Error())
		return
	}
	s.deps.Logger.Info("手动重载配置成功", "user", sess.Username)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type authChangeRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleConfigAuth 修改登录凭据（写回 config.yaml 并热更新）。
func (s *Service) handleConfigAuth(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req authChangeRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || strings.ContainsAny(req.Username, "\r\n") {
		writeErr(w, http.StatusBadRequest, CodeConfigInvalid, "用户名不能为空或包含换行")
		return
	}
	if len(req.Password) < config.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeConfigInvalid,
			fmt.Sprintf("密码长度不得少于 %d 个字符", config.MinPasswordLen))
		return
	}

	cfg := s.deps.State.Get()
	if err := config.SetAuth(cfg.Path, req.Username, req.Password); err != nil {
		s.deps.Logger.Error("写入登录凭据失败", "user", sess.Username, "error", err)
		writeErr(w, http.StatusInternalServerError, CodeInternal, "保存失败: "+err.Error())
		return
	}
	if s.deps.ReloadConfig != nil {
		if err := s.deps.ReloadConfig(); err != nil {
			writeErr(w, http.StatusInternalServerError, CodeInternal, "配置已写入但热加载失败: "+err.Error())
			return
		}
	}
	s.deps.Logger.Info("登录凭据已更新", "user", sess.Username, "target", req.Username)
	writeJSON(w, http.StatusOK, map[string]string{"username": req.Username})
}
