package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"dcc/internal/config"
	"dcc/internal/docker"
	"dcc/internal/jobs"
	"dcc/web"
)

// fakeDocker 构造伪 Docker 守护进程。
func fakeDocker(t *testing.T) *docker.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/_ping":
			fmt.Fprint(w, "OK")
		case p == "/version":
			_ = json.NewEncoder(w).Encode(docker.VersionResponse{Version: "24.0.7", APIVersion: "1.43"})
		case strings.HasSuffix(p, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]docker.ContainerSummary{
				{ID: "c1", Names: []string{"/web"}, Image: "nginx:latest", ImageID: "sha256:aaaa", State: "running"},
			})
		case strings.Contains(p, "/containers/create"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "c-new", "Warnings": []string{}})
		case strings.Contains(p, "/containers/") && strings.HasSuffix(p, "/json"):
			_ = json.NewEncoder(w).Encode(docker.ContainerInspect{
				ID:   "c1",
				Name: "/web",
				Config: docker.ContainerConfig{
					Image: "nginx:latest",
					Labels: map[string]string{"app": "web"},
				},
				State: docker.ContainerState{Status: "running", Running: true},
			})
		case strings.Contains(p, "/containers/"):
			w.WriteHeader(http.StatusNoContent) // start/stop/restart/rename/delete
		case strings.HasSuffix(p, "/images/create"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"status":"Status: Image is up to date for nginx:latest"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
		}
	}))
	t.Cleanup(srv.Close)

	client, err := docker.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("创建伪 Docker 客户端失败: %v", err)
	}
	return client
}

// testServer 启动完整的测试服务。
func testServer(t *testing.T) (*httptest.Server, *config.Config) {
	t.Helper()
	cfg := config.NewDefault()
	cfg.Auth.Username = "admin"
	cfg.Auth.Password = "test-password-123"
	cfg.Path = t.TempDir() + "/config.yaml"
	cfg.BaseDir = t.TempDir()

	client := fakeDocker(t)
	svc := New(&Deps{
		State:   NewState(cfg),
		Jobs:    jobs.NewManager(nil),
		Docker:  func() *docker.Client { return client },
		Compose: func() *docker.ComposeRunner { return docker.NewComposeRunner("") },
		Version: "test",
		Started: time.Now(),
	})

	ts := httptest.NewServer(svc.Handler())
	t.Cleanup(ts.Close)
	return ts, cfg
}

// loginClient 携带 Cookie 的客户端。
func loginClient(t *testing.T, ts *httptest.Server, user, pass string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	resp, err := client.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("登录请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("登录失败，状态码 %d", resp.StatusCode)
	}
	return client
}

// csrfToken 从会话接口读取 CSRF Token。
func csrfToken(t *testing.T, client *http.Client, ts *httptest.Server) string {
	t.Helper()
	resp, err := client.Get(ts.URL + "/api/session")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Authenticated bool   `json:"authenticated"`
			CSRFToken     string `json:"csrfToken"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Data.Authenticated {
		t.Fatal("应当已登录")
	}
	return payload.Data.CSRFToken
}

func TestLoginFlowAndAuth(t *testing.T) {
	ts, _ := testServer(t)

	// 未登录访问受保护接口。
	resp, err := http.Get(ts.URL + "/api/containers")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", resp.StatusCode)
	}

	// 错误密码。
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	resp, err = http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误密码应返回 401，实际 %d", resp.StatusCode)
	}

	// 正确登录。
	client := loginClient(t, ts, "admin", "test-password-123")
	token := csrfToken(t, client, ts)

	// 登录后可访问容器列表。
	resp, err = client.Get(ts.URL + "/api/containers")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("容器列表应返回 200，实际 %d", resp.StatusCode)
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Containers []containerItem `json:"containers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Containers) != 1 || payload.Data.Containers[0].ID != "c1" {
		t.Fatalf("容器数据错误: %+v", payload.Data.Containers)
	}
	if !payload.Data.Containers[0].IsLatest {
		t.Fatal("nginx:latest 应标记为 latest")
	}

	// 无 CSRF Token 的写操作应被拒绝。
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/containers/c1/restart", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("缺少 CSRF 应返回 403，实际 %d", resp.StatusCode)
	}

	// 带 CSRF Token 的写操作成功。
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/containers/c1/restart", nil)
	req.Header.Set(csrfHeaderName, token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("带 CSRF 的写操作应成功，实际 %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts, _ := testServer(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})

	var last int
	for i := 0; i < loginMaxFailures+2; i++ {
		resp, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		last = resp.StatusCode
		_ = resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("多次失败后应限流 429，实际 %d", last)
	}
}

func TestSecurityHeadersAndStatic(t *testing.T) {
	ts, _ := testServer(t)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("首页应返回 200，实际 %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("首页应返回 HTML，实际 %s", ct)
	}
	if resp.Header.Get("Content-Security-Policy") == "" ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" ||
		resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("缺少安全响应头: %+v", resp.Header)
	}

	// SPA 路由回退。
	resp2, err := http.Get(ts.URL + "/containers")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("SPA 路由应回退 index.html，实际 %d", resp2.StatusCode)
	}

	// 未匹配 API 返回 JSON 404。
	resp3, err := http.Get(ts.URL + "/api/unknown")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("未知 API 应返回 404，实际 %d", resp3.StatusCode)
	}
	if ct := resp3.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("未知 API 应返回 JSON，实际 %s", ct)
	}

	// 路径穿越防护。
	resp4, err := http.Get(ts.URL + "/../../internal/server/server.go")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("非法路径应回退 SPA，实际 %d", resp4.StatusCode)
	}
}

func TestStaticFilesEmbedded(t *testing.T) {
	spa, err := newSPAHandler(web.Static)
	if err != nil {
		t.Fatalf("前端资源未构建: %v", err)
	}
	if len(spa.index) == 0 {
		t.Fatal("index.html 为空")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/js/main.js", nil)
	spa.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("JS 资源应可访问，实际 %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("JS Content-Type 错误: %s", ct)
	}
}

func TestContainerUpdateLatest(t *testing.T) {
	ts, _ := testServer(t)
	client := loginClient(t, ts, "admin", "test-password-123")
	token := csrfToken(t, client, ts)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/containers/c1/update", nil)
	req.Header.Set(csrfHeaderName, token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("latest 容器应允许更新，实际 %d", resp.StatusCode)
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			JobID string `json:"jobId"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.JobID == "" {
		t.Fatal("应返回任务 ID")
	}
}
