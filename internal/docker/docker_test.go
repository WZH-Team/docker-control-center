package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		ref      string
		name     string
		tag      string
		wantErr  bool
		isLatest bool
	}{
		{ref: "nginx", name: "nginx", tag: "latest", isLatest: true},
		{ref: "nginx:1.25", name: "nginx", tag: "1.25", isLatest: false},
		{ref: "nginx:latest", name: "nginx", tag: "latest", isLatest: true},
		{ref: "registry.io:5000/app:v1", name: "registry.io:5000/app", tag: "v1"},
		{ref: "registry.io:5000/app", name: "registry.io:5000/app", tag: "latest", isLatest: true},
		{ref: "nginx@sha256:abc", name: "nginx", tag: "sha256:abc", isLatest: false},
		{ref: "nginx:tag@sha256:abc", name: "nginx:tag", tag: "sha256:abc", isLatest: false},
		{ref: "", wantErr: true},
		{ref: "bad name", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			name, tag, err := ParseImageRef(tc.ref)
			if tc.wantErr {
				if err == nil {
					t.Fatal("应当报错")
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if name != tc.name || tag != tc.tag {
				t.Fatalf("解析结果 %q:%q != %q:%q", name, tag, tc.name, tc.tag)
			}
			if got := RefIsLatest(tc.ref); got != tc.isLatest {
				t.Fatalf("RefIsLatest = %v, 期望 %v", got, tc.isLatest)
			}
		})
	}
}

func TestValidateDockerID(t *testing.T) {
	valid := []string{"abc123", "sha256:abcdef", "nginx:1.25", "my_app.v2", "registry:5000/a@sha256:b"}
	for _, id := range valid {
		if err := ValidateDockerID(id); err != nil {
			t.Fatalf("%q 应当合法: %v", id, err)
		}
	}
	invalid := []string{"", "a b", "a?b", "a\nb", strings.Repeat("a", 300)}
	for _, id := range invalid {
		if err := ValidateDockerID(id); err == nil {
			t.Fatalf("%q 应当非法", id)
		}
	}
}

// fakeDaemon 构造一个模拟 Docker 守护进程。
func fakeDaemon(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	return c
}

func TestNegotiationAndListContainers(t *testing.T) {
	var gotVersionedPath string
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "OK")
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(VersionResponse{Version: "24.0.7", APIVersion: "1.43", MinAPIVersion: "1.24"})
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			gotVersionedPath = r.URL.Path
			_ = json.NewEncoder(w).Encode([]ContainerSummary{{ID: "c1", Names: []string{"/web"}, Image: "nginx:latest", State: "running"}})
		default:
			http.NotFound(w, r)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := c.EnsureReady(ctx)
	if err != nil {
		t.Fatalf("EnsureReady 失败: %v", err)
	}
	if v.Version != "24.0.7" || c.APIVersion() != "1.43" {
		t.Fatalf("版本协商失败: %+v", v)
	}

	list, err := c.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers 失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != "c1" {
		t.Fatalf("容器列表错误: %+v", list)
	}
	if gotVersionedPath != "/v1.43/containers/json" {
		t.Fatalf("应使用版本化路径，实际 %s", gotVersionedPath)
	}
}

func TestAPIErrors(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "No such container: xyz"})
	})

	err := c.StartContainer(context.Background(), "xyz")
	if err == nil {
		t.Fatal("应当返回错误")
	}
	if !IsNotFound(err) {
		t.Fatalf("应识别为 404: %v", err)
	}
	var apiErr *APIError
	if !strings.Contains(fmt.Sprint(err), "No such container") {
		t.Fatalf("错误信息应包含守护进程消息: %v", err)
	}
	if !strings.Contains(fmt.Sprint(err), "404") {
		t.Fatalf("错误信息应包含状态码: %v", err)
	}
	_ = apiErr
}

func TestPullImageStream(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.43/images/create" && r.URL.Path != "/images/create" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("fromImage"); got != "nginx" {
			t.Errorf("fromImage 应为 nginx，实际 %q", got)
		}
		if got := r.URL.Query().Get("tag"); got != "latest" {
			t.Errorf("tag 应为 latest，实际 %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"Pulling from library/nginx","id":"latest"}`)
		fmt.Fprintln(w, `{"status":"Downloading","progressDetail":{"current":1,"total":2},"progress":"[==]"}`)
		fmt.Fprintln(w, `{"status":"Digest: sha256:deadbeef"}`)
		fmt.Fprintln(w, `{"status":"Status: Downloaded newer image for nginx:latest"}`)
	})

	var out strings.Builder
	if err := c.PullImage(context.Background(), "nginx:latest", &out); err != nil {
		t.Fatalf("PullImage 失败: %v", err)
	}
	log := out.String()
	if !strings.Contains(log, "Pulling from library/nginx") || !strings.Contains(log, "Digest: sha256:deadbeef") {
		t.Fatalf("输出缺少里程碑信息:\n%s", log)
	}
	if strings.Contains(log, "Downloading") {
		t.Fatalf("进度帧不应写入输出:\n%s", log)
	}
}

func TestPullImageError(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}`)
	})

	err := c.PullImage(context.Background(), "missing:latest", &strings.Builder{})
	if err == nil {
		t.Fatal("拉取失败应返回错误")
	}
	if !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("错误信息应包含守护进程消息: %v", err)
	}
}

func TestImageUsage(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			_ = json.NewEncoder(w).Encode([]ContainerSummary{
				{ID: "c1", Image: "nginx:latest", ImageID: "sha256:aaaa"},
				{ID: "c2", Image: "redis", ImageID: "sha256:bbbb"},
				{ID: "c3", Image: "nginx:old", ImageID: "aaaa"}, // 无前缀形式
			})
			return
		}
		http.NotFound(w, r)
	})

	used, err := c.ImageUsage(context.Background(), "sha256:aaaa")
	if err != nil {
		t.Fatalf("ImageUsage 失败: %v", err)
	}
	if len(used) != 2 {
		t.Fatalf("应识别出 2 个使用中的容器，实际 %d", len(used))
	}
}

func TestBuildRecreateRequest(t *testing.T) {
	inspect := &ContainerInspect{
		ID:   "c1",
		Name: "/web",
		Config: ContainerConfig{
			Image:      "nginx:latest",
			Env:        []string{"A=1"},
			Cmd:        []string{"nginx", "-g", "daemon off;"},
			Labels:     map[string]string{"app": "web"},
			ExposedPorts: map[string]struct{}{"80/tcp": {}},
		},
		HostConfig: HostConfig{
			Binds:        []string{"/data:/data"},
			RestartPolicy: RestartPolicy{Name: "always"},
			PortBindings: map[string][]PortBinding{"80/tcp": {{HostPort: "8080"}}},
			NetworkMode:  "my-net",
		},
		NetworkSettings: NetworkSettings{
			Networks: map[string]*EndpointSettings{
				"my-net": {
					IPAddress: "172.18.0.2",
					Aliases:   []string{"web"},
					IPAMConfig: &EndpointIPAMConfig{IPv4Address: "172.18.0.2"},
				},
			},
		},
		Mounts: []MountPoint{{Type: "volume", Name: "data-vol", Destination: "/data", RW: true}},
	}

	req := BuildRecreateRequest(inspect, "nginx:1.25")
	if req.Config.Image != "nginx:1.25" {
		t.Fatalf("新镜像未生效: %s", req.Config.Image)
	}
	if req.HostConfig.RestartPolicy.Name != "always" || len(req.HostConfig.Binds) != 1 {
		t.Fatalf("HostConfig 未正确保留: %+v", req.HostConfig)
	}
	if req.NetworkingConfig == nil || req.NetworkingConfig.EndpointsConfig["my-net"] == nil {
		t.Fatal("网络端点未重建")
	}
	ep := req.NetworkingConfig.EndpointsConfig["my-net"]
	if ep.IPAddress != "" || ep.IPAMConfig == nil || ep.IPAMConfig.IPv4Address != "172.18.0.2" {
		t.Fatalf("端点配置错误: %+v", ep)
	}

	// 默认 bridge 网络不应携带静态 IP 与别名。
	inspect.NetworkSettings.Networks = map[string]*EndpointSettings{
		"bridge": {IPAddress: "172.17.0.2", Aliases: []string{"x"}},
	}
	inspect.HostConfig.NetworkMode = "default"
	req2 := BuildRecreateRequest(inspect, "nginx:1.25")
	ep2 := req2.NetworkingConfig.EndpointsConfig["bridge"]
	if ep2 == nil || ep2.IPAMConfig != nil || len(ep2.Aliases) != 0 {
		t.Fatalf("默认网络端点应剔除静态配置: %+v", ep2)
	}
}
