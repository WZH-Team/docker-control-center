package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
	"time"

	"dcc/internal/config"
)

const (
	sessionCookieName = "dcc_sid"
	csrfHeaderName    = "X-CSRF-Token"
	sessionTTL        = 24 * time.Hour
	maxSessions       = 1000
	loginWindow       = 5 * time.Minute
	loginMaxFailures  = 10
)

// Session 登录会话。
type Session struct {
	ID        string
	CSRF      string
	Username  string
	CreatedAt time.Time
	LastSeen  time.Time
}

// SessionManager 会话管理器（内存态；进程重启后需重新登录）。
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewSessionManager 创建会话管理器。
func NewSessionManager() *SessionManager {
	sm := &SessionManager{sessions: make(map[string]*Session)}
	go sm.cleanupLoop()
	return sm
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败属致命错误，退化为时间戳随机性不足以支撑安全会话。
		panic("crypto/rand 不可用: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Create 创建新会话。
func (sm *SessionManager) Create(username string) *Session {
	now := time.Now()
	s := &Session{
		ID:        randomToken(32),
		CSRF:      randomToken(32),
		Username:  username,
		CreatedAt: now,
		LastSeen:  now,
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	// 数量兜底：淘汰最久未活动的会话。
	for len(sm.sessions) >= maxSessions {
		var oldestID string
		var oldest time.Time
		for id, s := range sm.sessions {
			if oldestID == "" || s.LastSeen.Before(oldest) {
				oldestID, oldest = id, s.LastSeen
			}
		}
		if oldestID == "" {
			break
		}
		delete(sm.sessions, oldestID)
	}
	sm.sessions[s.ID] = s
	return s
}

// Get 查找会话并刷新活动时间。
func (sm *SessionManager) Get(id string) (*Session, bool) {
	if id == "" {
		return nil, false
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[id]
	if !ok {
		return nil, false
	}
	if time.Since(s.CreatedAt) > sessionTTL {
		delete(sm.sessions, id)
		return nil, false
	}
	s.LastSeen = time.Now()
	return s, true
}

// Delete 删除会话。
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
	sm.mu.Unlock()
}

// DeleteUser 删除某用户的所有会话（改密码后调用）。
func (sm *SessionManager) DeleteUser(username string) {
	sm.mu.Lock()
	for id, s := range sm.sessions {
		if s.Username == username {
			delete(sm.sessions, id)
		}
	}
	sm.mu.Unlock()
}

func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		sm.mu.Lock()
		for id, s := range sm.sessions {
			if time.Since(s.CreatedAt) > sessionTTL {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

// verifyCredentials 常量时间比较用户名与密码，防止时序侧信道。
func verifyCredentials(cfg *config.Config, username, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(cfg.Auth.Username), []byte(username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(cfg.Auth.Password), []byte(password)) == 1
	return userOK && passOK
}

// loginLimiter 登录失败限流器（按 IP + 用户名滑动窗口）。
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	l := &loginLimiter{attempts: make(map[string][]time.Time)}
	go l.cleanupLoop()
	return l
}

func (l *loginLimiter) key(ip, username string) string { return ip + "|" + username }

// Allow 判断是否允许尝试登录。
func (l *loginLimiter) Allow(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := l.key(ip, username)
	now := time.Now()
	cutoff := now.Add(-loginWindow)
	times := l.attempts[k][:0]
	for _, t := range l.attempts[k] {
		if t.After(cutoff) {
			times = append(times, t)
		}
	}
	l.attempts[k] = times
	return len(times) < loginMaxFailures
}

// Fail 记录一次失败尝试。
func (l *loginLimiter) Fail(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := l.key(ip, username)
	l.attempts[k] = append(l.attempts[k], time.Now())
}

// Reset 清空计数（登录成功后）。
func (l *loginLimiter) Reset(ip, username string) {
	l.mu.Lock()
	delete(l.attempts, l.key(ip, username))
	l.mu.Unlock()
}

func (l *loginLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		cutoff := time.Now().Add(-loginWindow)
		for k, times := range l.attempts {
			kept := times[:0]
			for _, t := range times {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(l.attempts, k)
			} else {
				l.attempts[k] = kept
			}
		}
		l.mu.Unlock()
	}
}

// clientIP 提取客户端 IP（信任 X-Forwarded-For 的第一跳，便于反代部署）。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := firstIP(xff); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func firstIP(xff string) string {
	for i := 0; i < len(xff); i++ {
		if xff[i] == ',' {
			return trimSpace(xff[:i])
		}
	}
	return trimSpace(xff)
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// setSessionCookie 写入会话 Cookie（HttpOnly + SameSite=Lax，HTTPS 下附加 Secure）。
func setSessionCookie(w http.ResponseWriter, r *http.Request, cfg *config.Config, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.SSL.Enabled || r.TLS != nil,
		MaxAge:   int(sessionTTL / time.Second),
	})
}

// clearSessionCookie 清除会话 Cookie。
func clearSessionCookie(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.SSL.Enabled || r.TLS != nil,
		MaxAge:   -1,
	})
}
