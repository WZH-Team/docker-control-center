// Package docker 实现轻量级 Docker Engine API 客户端（仅依赖标准库）。
//
// 连接方式：
//   - Linux（默认）: unix:///var/run/docker.sock
//   - 自定义: 支持 unix://、tcp://、http://、https://（https 读取 DOCKER_CERT_PATH 证书）
//
// API 版本兼容：首次连接时通过 /version 协商守护进程 API 版本并按需降级，
// 未协商成功时使用无版本前缀路径，从而兼容不同版本的 Docker 引擎。
package docker

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Client Docker Engine API 客户端。
type Client struct {
	http       *http.Client
	baseURL    string
	host       string
	apiVersion string // 协商后的 API 版本，如 "1.43"；空表示使用无版本路径
}

// APIError Docker 守护进程返回的错误。
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Docker API 错误 (%d): %s", e.StatusCode, e.Message)
}

// IsNotFound 判断是否为资源不存在错误。
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsConflict 判断是否为资源冲突错误（如镜像/存储卷被占用）。
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

// IsNotModified 判断是否为 304（操作无须执行）。
func IsNotModified(err error) bool { return hasStatus(err, http.StatusNotModified) }

func hasStatus(err error, code int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == code
}

// DefaultHost 返回当前平台默认的 Docker 守护进程地址。
func DefaultHost() string {
	if v := strings.TrimSpace(os.Getenv("DOCKER_HOST")); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		// Windows 无原生 unix socket 支持，需显式配置（如 Docker Desktop 开启 tcp://127.0.0.1:2375）。
		return ""
	}
	return "unix:///var/run/docker.sock"
}

// NewClient 创建 Docker 客户端。host 为空时使用 DOCKER_HOST 或平台默认地址。
func NewClient(host string) (*Client, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = DefaultHost()
	}
	if host == "" {
		return nil, errors.New("未配置 Docker 地址：请在 config.yaml 的 docker.host 中配置（Windows 开发环境示例：tcp://127.0.0.1:2375）")
	}

	c := &Client{host: host}
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	switch {
	case strings.HasPrefix(host, "unix://"):
		socket := strings.TrimPrefix(host, "unix://")
		c.baseURL = "http://docker"
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "unix", socket)
		}
	case strings.HasPrefix(host, "npipe://"):
		return nil, errors.New("暂不支持 npipe 连接，请改用 tcp:// 地址（如 Docker Desktop 开启 tcp://127.0.0.1:2375）")
	case strings.HasPrefix(host, "tcp://"):
		c.baseURL = "http://" + strings.TrimPrefix(host, "tcp://")
		if err := setupClientTLS(transport, strings.TrimPrefix(host, "tcp://")); err != nil {
			return nil, err
		}
	case strings.HasPrefix(host, "http://"), strings.HasPrefix(host, "https://"):
		c.baseURL = host
		if strings.HasPrefix(host, "https://") {
			if err := setupClientTLS(transport, strings.TrimPrefix(host, "https://")); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("无法识别的 Docker 地址: %q（支持 unix:// tcp:// http:// https://）", host)
	}

	c.http = &http.Client{Transport: transport}
	return c, nil
}

// setupClientTLS 为 https/tcp 连接加载客户端证书（DOCKER_CERT_PATH）。
func setupClientTLS(tr *http.Transport, addr string) error {
	// 未配置证书目录时尝试系统信任链 + 可选客户端证书。
	certPath := strings.TrimSpace(os.Getenv("DOCKER_CERT_PATH"))
	if certPath == "" {
		// 默认尝试 ~/.docker
		if home, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(home, ".docker")
			if _, err := os.Stat(filepath.Join(candidate, "ca.pem")); err == nil {
				certPath = candidate
			}
		}
	}
	if certPath == "" {
		return nil // 使用系统信任链，无客户端证书
	}

	caData, err := os.ReadFile(filepath.Join(certPath, "ca.pem"))
	if err != nil {
		return fmt.Errorf("读取 Docker CA 证书失败: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return errors.New("解析 Docker CA 证书失败")
	}
	tlsCfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if cert, err := tls.LoadX509KeyPair(filepath.Join(certPath, "cert.pem"), filepath.Join(certPath, "key.pem")); err == nil {
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	tr.TLSClientConfig = tlsCfg
	// 校验证书主机名与地址一致。
	_ = addr
	return nil
}

// Host 返回配置的 Docker 地址。
func (c *Client) Host() string { return c.host }

// APIVersion 返回协商后的 API 版本（可能为空）。
func (c *Client) APIVersion() string { return c.apiVersion }

// path 生成带版本前缀的 API 路径。
func (c *Client) path(p string) string {
	if c.apiVersion != "" {
		return "/v" + c.apiVersion + p
	}
	return p
}

// do 发起 API 请求并返回原始响应（调用方负责关闭 Body）。
func (c *Client) do(ctx context.Context, method, p string, query url.Values, body any, headers map[string]string) (*http.Response, error) {
	full := c.baseURL + c.path(p)
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, full, reader)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "DCC/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 Docker 守护进程失败: %w", err)
	}
	return resp, nil
}

// decodeError 将非 2xx 响应转换为 APIError。
func decodeError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	msg := strings.TrimSpace(string(data))
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &payload); err == nil && payload.Message != "" {
		msg = payload.Message
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

// doJSON 发起请求并将 2xx 响应体解析到 out（out 可为 nil）。
func (c *Client) doJSON(ctx context.Context, method, p string, query url.Values, body, out any) error {
	resp, err := c.do(ctx, method, p, query, body, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("解析 Docker 响应失败: %w", err)
	}
	return nil
}

// Ping 检测守护进程是否可用。
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/_ping", nil, nil, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return nil
}

// ServerVersion 获取守护进程版本信息，并完成 API 版本协商。
func (c *Client) ServerVersion(ctx context.Context) (*VersionResponse, error) {
	var v VersionResponse
	if err := c.doJSON(ctx, http.MethodGet, "/version", nil, nil, &v); err != nil {
		return nil, err
	}
	if v.APIVersion != "" {
		c.apiVersion = v.APIVersion
	}
	return &v, nil
}

// EnsureReady 确保与守护进程完成握手（Ping + 版本协商）。
func (c *Client) EnsureReady(ctx context.Context) (*VersionResponse, error) {
	if err := c.Ping(ctx); err != nil {
		return nil, err
	}
	v, err := c.ServerVersion(ctx)
	if err != nil {
		// 版本接口异常时退化为无版本路径，仍可继续使用。
		c.apiVersion = ""
		return nil, err
	}
	return v, nil
}

// ---------- 镜像引用解析 ----------

// ParseImageRef 将镜像引用拆分为拉取接口需要的 name 与 tag/digest。
// 支持：nginx、nginx:1.25、registry.io:5000/app:v1、nginx@sha256:xxx、nginx:tag@sha256:xxx
func ParseImageRef(ref string) (name, tag string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", errors.New("镜像名称不能为空")
	}
	if strings.ContainsAny(ref, " \t\n") {
		return "", "", errors.New("镜像名称不能包含空白字符")
	}

	// digest 优先。
	if at := strings.LastIndex(ref, "@"); at > 0 {
		name = ref[:at]
		tag = ref[at+1:]
		if name == "" || tag == "" {
			return "", "", fmt.Errorf("无效的镜像引用: %q", ref)
		}
		return name, tag, nil
	}

	// tag：最后一个 ':' 且位于最后一个 '/' 之后。
	name = ref
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		name = ref[:colon]
		tag = ref[colon+1:]
	}
	if name == "" {
		return "", "", fmt.Errorf("无效的镜像引用: %q", ref)
	}
	if tag == "" {
		tag = "latest"
	}
	return name, tag, nil
}

// RefIsLatest 判断镜像引用是否指向 latest 标签（含无标签的隐式 latest）。
// digest 固定的引用（含 @sha256:）不视为 latest。
func RefIsLatest(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "@") {
		return false
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		return ref[colon+1:] == "latest"
	}
	return true // 无标签 = 隐式 latest
}

// TrimImageID 规范化镜像 ID（去掉 sha256: 前缀后比较）。
func TrimImageID(id string) string {
	return strings.TrimPrefix(strings.TrimSpace(id), "sha256:")
}

// ---------- 私有仓库认证（读取 ~/.docker/config.json，尽力而为） ----------

type dockerCLIConfig struct {
	Auths map[string]struct {
		Auth     string `json:"auth"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auths"`
}

// registryAuthHeader 构造 X-Registry-Auth 请求头（base64url 编码的 AuthConfig JSON）。
func registryAuthHeader(imageName string) string {
	cfgPath := os.Getenv("DOCKER_CONFIG")
	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		cfgPath = filepath.Join(home, ".docker")
	}
	data, err := os.ReadFile(filepath.Join(cfgPath, "config.json"))
	if err != nil {
		return ""
	}
	var cfg dockerCLIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}

	registry := registryOf(imageName)
	candidates := []string{
		registry,
		"https://" + registry,
		registry + "/v1/",
		"https://" + registry + "/v1/",
	}
	if registry == "registry-1.docker.io" || registry == "index.docker.io" {
		candidates = append(candidates,
			"https://index.docker.io/v1/", "index.docker.io", "registry-1.docker.io", "docker.io")
	}

	for _, key := range candidates {
		entry, ok := cfg.Auths[key]
		if !ok {
			continue
		}
		user, pass := entry.Username, entry.Password
		if entry.Auth != "" && (user == "" || pass == "") {
			raw, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err == nil {
				if i := strings.IndexByte(string(raw), ':'); i > 0 {
					user, pass = string(raw[:i]), string(raw[i+1:])
				}
			}
		}
		if user == "" && pass == "" {
			continue
		}
		payload, err := json.Marshal(map[string]string{
			"username":      user,
			"password":      pass,
			"serveraddress": key,
		})
		if err != nil {
			return ""
		}
		return base64.URLEncoding.EncodeToString(payload)
	}
	return ""
}

// registryOf 从镜像名中提取仓库主机名。
func registryOf(name string) string {
	first := name
	if i := strings.IndexByte(first, '/'); i > 0 {
		first = first[:i]
	}
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return first
	}
	return "registry-1.docker.io"
}
