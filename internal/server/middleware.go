package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// securityHeaders 统一安全响应头。
// script-src 严格限制为同源；样式允许内联（进度条等动态样式），风险可控。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
				"connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// withRecover panic 兜底，避免单请求崩溃影响服务。
func (s *Service) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.deps.Logger.Error("请求处理发生 panic",
					"method", r.Method, "path", r.URL.Path,
					"panic", rec, "stack", string(debug.Stack()))
				writeErr(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder 记录响应状态码。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

// withRequestLog 记录 API 访问日志（不含请求体，避免泄露敏感信息）。
func (s *Service) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) < 4 || r.URL.Path[:4] != "/api" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.deps.Logger.Info("HTTP 请求",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"ip", clientIP(r), "duration", time.Since(start).String())
	})
}

// sessionFromRequest 从 Cookie 解析会话。
func (s *Service) sessionFromRequest(r *http.Request) (*Session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	return s.sessions.Get(cookie.Value)
}

// authedFunc 需要登录的处理函数签名。
type authedFunc func(w http.ResponseWriter, r *http.Request, sess *Session)

// requireAuth 包装需要登录鉴权的处理函数。
func (s *Service) requireAuth(next authedFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.sessionFromRequest(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "未登录或会话已过期")
			return
		}
		next(w, r, sess)
	}
}

// mutatingMethods 需要 CSRF 校验的请求方法。
func mutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// checkCSRF 校验 CSRF Token（写操作防护）。
func checkCSRF(r *http.Request, sess *Session) bool {
	got := r.Header.Get(csrfHeaderName)
	if got == "" || sess == nil {
		return false
	}
	return constantTimeEqual(got, sess.CSRF)
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// ensureLogger 防止 nil logger（测试场景）。
func ensureLogger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}
