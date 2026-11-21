package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ListContainers 列出容器（all=true 包含已停止容器）。
func (c *Client) ListContainers(ctx context.Context, all bool) ([]ContainerSummary, error) {
	q := url.Values{}
	if all {
		q.Set("all", "true")
	}
	var out []ContainerSummary
	if err := c.doJSON(ctx, http.MethodGet, "/containers/json", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// InspectContainer 获取容器详细信息。
func (c *Client) InspectContainer(ctx context.Context, id string) (*ContainerInspect, error) {
	var out ContainerInspect
	if err := c.doJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartContainer 启动容器。
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, nil, nil)
}

// StopContainer 停止容器（timeout 秒后强制结束；<=0 使用守护进程默认值）。
func (c *Client) StopContainer(ctx context.Context, id string, timeoutSec int) error {
	q := url.Values{}
	if timeoutSec > 0 {
		q.Set("t", strconv.Itoa(timeoutSec))
	}
	return c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop", q, nil, nil)
}

// RestartContainer 重启容器。
func (c *Client) RestartContainer(ctx context.Context, id string, timeoutSec int) error {
	q := url.Values{}
	if timeoutSec > 0 {
		q.Set("t", strconv.Itoa(timeoutSec))
	}
	return c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/restart", q, nil, nil)
}

// RemoveContainer 删除容器。
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "true")
	}
	return c.doJSON(ctx, http.MethodDelete, "/containers/"+url.PathEscape(id), q, nil, nil)
}

// RenameContainer 重命名容器。
func (c *Client) RenameContainer(ctx context.Context, id, newName string) error {
	q := url.Values{}
	q.Set("name", newName)
	return c.doJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/rename", q, nil, nil)
}

// CreateContainer 创建容器并返回容器 ID。
func (c *Client) CreateContainer(ctx context.Context, name string, req ContainerCreateRequest) (*ContainerCreateResponse, error) {
	q := url.Values{}
	if name != "" {
		q.Set("name", name)
	}
	var out ContainerCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, "/containers/create", q, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BuildRecreateRequest 基于容器 inspect 结果构造“重建容器”请求（用于容器更新）。
// newImageRef 为重建时使用的新镜像引用。
func BuildRecreateRequest(inspect *ContainerInspect, newImageRef string) ContainerCreateRequest {
	cfg := inspect.Config
	cfg.Image = newImageRef

	// 创建时清空运行期字段，避免报错。
	cfg.AttachStdin, cfg.AttachStdout, cfg.AttachStderr = false, false, false

	host := inspect.HostConfig

	// 挂载重建策略：优先保留创建时的 Binds 与 Mounts；
	// 若二者为空但运行期存在挂载点（旧容器场景），则由 MountPoint 反推。
	if len(host.Binds) == 0 && len(host.Mounts) == 0 && len(inspect.Mounts) > 0 {
		host.Mounts = mountsFromPoints(inspect.Mounts)
	}

	req := ContainerCreateRequest{
		Config:     cfg,
		HostConfig: host,
	}

	// 网络重建：仅保留创建时可用字段。
	if !isSpecialNetworkMode(host.NetworkMode) && len(inspect.NetworkSettings.Networks) > 0 {
		eps := make(map[string]*EndpointSettings, len(inspect.NetworkSettings.Networks))
		for name, ep := range inspect.NetworkSettings.Networks {
			if ep == nil {
				continue
			}
			// 仅内置默认 bridge 网络需要剔除别名与静态 IP 配置。
			isDefault := name == "bridge" || name == "default"
			eps[name] = ep.CreateEndpoint(isDefault)
		}
		if len(eps) > 0 {
			req.NetworkingConfig = &NetworkingConfig{EndpointsConfig: eps}
		}
	}
	return req
}

// isSpecialNetworkMode 判断是否为不适用网络端点配置的网络模式。
func isSpecialNetworkMode(mode string) bool {
	return mode == "none" || mode == "host" ||
		len(mode) > 10 && (mode[:10] == "container:")
}

// mountsFromPoints 由 inspect 挂载点反推创建用的挂载配置。
func mountsFromPoints(points []MountPoint) []Mount {
	mounts := make([]Mount, 0, len(points))
	for _, mp := range points {
		m := Mount{Target: mp.Destination, ReadOnly: !mp.RW}
		switch mp.Type {
		case "bind":
			m.Type = "bind"
			m.Source = mp.Source
			if mp.Propagation != "" {
				m.BindOptions = &BindOptions{Propagation: mp.Propagation}
			}
		case "volume":
			m.Type = "volume"
			m.Source = mp.Name // 卷名
			if m.Source == "" {
				m.Source = mp.Source
			}
		case "tmpfs":
			m.Type = "tmpfs"
		default:
			continue
		}
		mounts = append(mounts, m)
	}
	return mounts
}

// ShortID 返回便于展示的短 ID。
func ShortID(id string) string {
	id = TrimImageID(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ContainerName 提取容器首选名称（去掉前导斜杠）。
func ContainerName(names []string, fallback string) string {
	if len(names) > 0 {
		name := names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
		return name
	}
	return fallback
}

// ValidateDockerID 校验容器/镜像 ID 或名称格式，防止路径注入。
func ValidateDockerID(id string) error {
	if id == "" {
		return fmt.Errorf("ID 不能为空")
	}
	if len(id) > 256 {
		return fmt.Errorf("ID 过长")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == ':' || r == '/' || r == '@' || r == '[' || r == ']':
		default:
			return fmt.Errorf("ID 包含非法字符 %q", r)
		}
	}
	return nil
}
