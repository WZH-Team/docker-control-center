package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dcc/internal/docker"
)

// portItem 端口映射。
type portItem struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort,omitempty"`
	Type        string `json:"type"`
}

// containerItem 容器列表条目。
type containerItem struct {
	ID                 string            `json:"id"`
	IDShort            string            `json:"idShort"`
	Name               string            `json:"name"`
	Image              string            `json:"image"`
	ImageID            string            `json:"imageId"`
	Command            string            `json:"command"`
	State              string            `json:"state"`
	Status             string            `json:"status"`
	Created            string            `json:"created"`
	Ports              []portItem        `json:"ports"`
	Labels             map[string]string `json:"labels"`
	IsLatest           bool              `json:"isLatest"`
	ComposeProject     string            `json:"composeProject"`
	ComposeWorkingDir  string            `json:"composeWorkingDir"`
}

func toPorts(ports []docker.Port) []portItem {
	out := make([]portItem, 0, len(ports))
	for _, p := range ports {
		out = append(out, portItem{
			IP:          p.IP,
			PrivatePort: p.PrivatePort,
			PublicPort:  p.PublicPort,
			Type:        p.Type,
		})
	}
	return out
}

func toContainerItem(ctr docker.ContainerSummary) containerItem {
	return containerItem{
		ID:                ctr.ID,
		IDShort:           docker.ShortID(ctr.ID),
		Name:              docker.ContainerName(ctr.Names, docker.ShortID(ctr.ID)),
		Image:             ctr.Image,
		ImageID:           ctr.ImageID,
		Command:           ctr.Command,
		State:             ctr.State,
		Status:            ctr.Status,
		Created:           formatUnix(ctr.Created),
		Ports:             toPorts(ctr.Ports),
		Labels:            ctr.Labels,
		IsLatest:          docker.RefIsLatest(ctr.Image),
		ComposeProject:    ctr.Labels["com.docker.compose.project"],
		ComposeWorkingDir: ctr.Labels["com.docker.compose.project.working_dir"],
	}
}

// handleContainersList 列出全部容器。
func (s *Service) handleContainersList(w http.ResponseWriter, r *http.Request, _ *Session) {
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	list, err := client.ListContainers(r.Context(), true)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	items := make([]containerItem, 0, len(list))
	for _, ctr := range list {
		items = append(items, toContainerItem(ctr))
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": items})
}

// mountItem 挂载信息。
type mountItem struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

// networkItem 网络信息。
type networkItem struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip,omitempty"`
	Gateway string   `json:"gateway,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

// containerDetail 容器详细信息。
type containerDetail struct {
	containerItem
	Entrypoint     []string          `json:"entrypoint"`
	Cmd            []string          `json:"cmd"`
	Env            []string          `json:"env"`
	WorkingDir     string            `json:"workingDir"`
	User           string            `json:"user"`
	Platform       string            `json:"platform"`
	StartedAt      string            `json:"startedAt"`
	FinishedAt     string            `json:"finishedAt"`
	ExitCode       int               `json:"exitCode"`
	OOMKilled      bool              `json:"oomKilled"`
	RestartCount   int               `json:"restartCount"`
	RestartPolicy  string            `json:"restartPolicy"`
	ExposedPorts   []string          `json:"exposedPorts"`
	Mounts         []mountItem       `json:"mounts"`
	Networks       []networkItem     `json:"networks"`
}

// handleContainerDetail 返回容器详情。
func (s *Service) handleContainerDetail(w http.ResponseWriter, r *http.Request, _ *Session) {
	id := r.PathValue("id")
	if err := docker.ValidateDockerID(id); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	inspect, err := client.InspectContainer(r.Context(), id)
	if err != nil {
		writeDockerErr(w, err)
		return
	}

	detail := containerDetail{
		containerItem: toContainerItem(docker.ContainerSummary{
			ID:      inspect.ID,
			Names:   []string{inspect.Name},
			Image:   inspect.Config.Image,
			ImageID: inspect.Image,
			Command: strings.Join(append([]string{inspect.Path}, inspect.Args...), " "),
			Labels:  inspect.Config.Labels,
			State:   inspect.State.Status,
			Status:  inspect.State.Status,
			Created: 0,
			Ports:   nil,
		}),
		Entrypoint:    inspect.Config.Entrypoint,
		Cmd:           inspect.Config.Cmd,
		Env:           inspect.Config.Env,
		WorkingDir:    inspect.Config.WorkingDir,
		User:          inspect.Config.User,
		Platform:      inspect.Platform,
		StartedAt:     inspect.State.StartedAt,
		FinishedAt:    inspect.State.FinishedAt,
		ExitCode:      inspect.State.ExitCode,
		OOMKilled:     inspect.State.OOMKilled,
		RestartCount:  inspect.RestartCount,
		RestartPolicy: inspect.HostConfig.RestartPolicy.Name,
	}
	detail.Created = inspect.Created

	for p := range inspect.Config.ExposedPorts {
		detail.ExposedPorts = append(detail.ExposedPorts, p)
	}
	for _, mp := range inspect.Mounts {
		detail.Mounts = append(detail.Mounts, mountItem{
			Type:        mp.Type,
			Name:        mp.Name,
			Source:      mp.Source,
			Destination: mp.Destination,
			RW:          mp.RW,
		})
	}
	for name, ep := range inspect.NetworkSettings.Networks {
		if ep == nil {
			continue
		}
		detail.Networks = append(detail.Networks, networkItem{
			Name:    name,
			IP:      ep.IPAddress,
			Gateway: ep.Gateway,
			Aliases: ep.Aliases,
		})
	}
	// 端口映射优先取 HostConfig 绑定。
	for port, bindings := range inspect.HostConfig.PortBindings {
		for _, b := range bindings {
			detail.Ports = append(detail.Ports, portItem{
				IP:          b.HostIP,
				PrivatePort: parsePort(port),
				PublicPort:  parsePort(b.HostPort),
				Type:        portType(port),
			})
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

func parsePort(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func portType(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return "tcp"
}

// containerAction 通用容器操作（启动/停止/重启）。
func (s *Service) containerAction(action string) authedFunc {
	return func(w http.ResponseWriter, r *http.Request, sess *Session) {
		id := r.PathValue("id")
		if err := docker.ValidateDockerID(id); err != nil {
			writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
			return
		}
		client, ok := s.dockerClient(w)
		if !ok {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		var err error
		switch action {
		case "start":
			err = client.StartContainer(ctx, id)
		case "stop":
			err = client.StopContainer(ctx, id, 15)
			if docker.IsNotModified(err) { // 已处于停止状态
				err = nil
			}
		case "restart":
			err = client.RestartContainer(ctx, id, 15)
		}
		if err != nil {
			s.deps.Logger.Warn("容器操作失败", "user", sess.Username, "action", action, "target", id, "error", err)
			writeDockerErr(w, err)
			return
		}
		s.deps.Logger.Info("容器操作成功", "user", sess.Username, "action", "container."+action, "target", id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// handleContainerDelete 删除容器。
func (s *Service) handleContainerDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id := r.PathValue("id")
	if err := docker.ValidateDockerID(id); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	force := r.URL.Query().Get("force") == "true"

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.RemoveContainer(ctx, id, force); err != nil {
		s.deps.Logger.Warn("删除容器失败", "user", sess.Username, "target", id, "error", err)
		writeDockerErr(w, err)
		return
	}
	s.deps.Logger.Info("删除容器成功", "user", sess.Username, "target", id, "force", force)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleContainerUpdate 更新容器：拉取 latest 镜像并以相同配置重建容器。
// 原容器会重命名保留为备份（停止状态），便于回滚。
func (s *Service) handleContainerUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	id := r.PathValue("id")
	if err := docker.ValidateDockerID(id); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	inspect, err := client.InspectContainer(ctx, id)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	ref := inspect.Config.Image
	if !docker.RefIsLatest(ref) {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, "仅支持标签为 latest 的容器执行更新")
		return
	}

	name := strings.TrimPrefix(inspect.Name, "/")
	s.deps.Logger.Info("开始更新容器", "user", sess.Username, "target", id, "image", ref)
	job := s.deps.Jobs.Start("container.update", fmt.Sprintf("更新容器 %s", name), func(jctx context.Context, out io.Writer) error {
		return s.runContainerUpdate(jctx, out, client, id, name, ref)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID})
}

// runContainerUpdate 执行容器更新流水线（拉取 → 停止 → 备份重命名 → 重建 → 启动）。
func (s *Service) runContainerUpdate(ctx context.Context, out io.Writer, client *docker.Client, id, name, ref string) error {
	fmt.Fprintf(out, "==> 拉取镜像 %s\n", ref)
	if err := client.PullImage(ctx, ref, out); err != nil {
		return err
	}

	// 拉取后重新 inspect，保证以最新容器配置重建。
	inspect, err := client.InspectContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("读取容器信息失败: %w", err)
	}

	backupName := fmt.Sprintf("%s.dccbak-%s", name, time.Now().Format("20060102-150405"))
	fmt.Fprintf(out, "==> 停止容器并备份为 %s\n", backupName)

	if err := client.StopContainer(ctx, id, 15); err != nil && !docker.IsNotModified(err) {
		return fmt.Errorf("停止容器失败: %w", err)
	}
	if err := client.RenameContainer(ctx, id, backupName); err != nil {
		return fmt.Errorf("备份原容器失败: %w", err)
	}

	req := docker.BuildRecreateRequest(inspect, ref)
	created, err := client.CreateContainer(ctx, name, req)
	if err != nil {
		_ = client.RenameContainer(ctx, id, name) // 回滚名称
		return fmt.Errorf("重建容器失败（已回滚原容器）: %w", err)
	}
	if err := client.StartContainer(ctx, created.ID); err != nil {
		_ = client.RemoveContainer(ctx, created.ID, true)
		_ = client.RenameContainer(ctx, id, name)
		return fmt.Errorf("启动新容器失败（已回滚原容器）: %w", err)
	}

	fmt.Fprintf(out, "==> 更新完成\n")
	fmt.Fprintf(out, "新容器 ID: %s\n", docker.ShortID(created.ID))
	fmt.Fprintf(out, "原容器已保留为 %s（停止状态），确认无误后可在容器列表中删除。\n", backupName)
	return nil
}

// dockerClient 获取 Docker 客户端，不可用时写入错误响应。
func (s *Service) dockerClient(w http.ResponseWriter) (*docker.Client, bool) {
	client := s.deps.Docker()
	if client == nil {
		writeErr(w, http.StatusServiceUnavailable, CodeDockerDown, "Docker 客户端不可用")
		return nil, false
	}
	return client, true
}
