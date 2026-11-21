package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"dcc/internal/config"
	"dcc/internal/docker"
)

// composeProjectItem Compose 项目视图。
type composeProjectItem struct {
	Name        string           `json:"name"`
	Path        string           `json:"path"`
	AbsPath     string           `json:"absPath"`
	Valid       bool             `json:"valid"`
	Error       string           `json:"error,omitempty"`
	ComposeFile string           `json:"composeFile,omitempty"`
	Total       int              `json:"total"`
	Running     int              `json:"running"`
	Stopped     int              `json:"stopped"`
	Containers  []containerBrief `json:"containers"`
}

// containerBrief 容器简要状态。
type containerBrief struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// handleComposeProjects 列出导入的 Compose 项目。
func (s *Service) handleComposeProjects(w http.ResponseWriter, r *http.Request, _ *Session) {
	cfg := s.deps.State.Get()

	var containers []docker.ContainerSummary
	if client, ok := s.dockerClient(w); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		list, err := client.ListContainers(ctx, true)
		cancel()
		if err == nil {
			containers = list
		}
	}

	items := make([]composeProjectItem, 0, len(cfg.ComposeProjects))
	for _, p := range cfg.ComposeProjects {
		abs := cfg.ComposeProjectPath(p)
		name := p.Name
		if name == "" {
			name = docker.NormalizeProjectName(abs)
		}
		item := composeProjectItem{
			Name:    name,
			Path:    p.Path,
			AbsPath: abs,
		}
		if composeFile, err := docker.ValidateProjectDir(abs); err != nil {
			item.Error = err.Error()
		} else {
			item.Valid = true
			item.ComposeFile = composeFile
		}

		for _, ctr := range containers {
			if !projectOwnsContainer(ctr.Labels, abs, name) {
				continue
			}
			state := ctr.State
			item.Total++
			if state == "running" {
				item.Running++
			} else {
				item.Stopped++
			}
			item.Containers = append(item.Containers, containerBrief{
				ID:    ctr.ID,
				Name:  docker.ContainerName(ctr.Names, docker.ShortID(ctr.ID)),
				State: state,
			})
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": items})
}

// projectOwnsContainer 判断容器是否属于指定 Compose 项目。
func projectOwnsContainer(labels map[string]string, absPath, name string) bool {
	if wd := labels["com.docker.compose.project.working_dir"]; wd != "" {
		return filepath.Clean(wd) == filepath.Clean(absPath)
	}
	return labels["com.docker.compose.project"] == name
}

type composeImportRequest struct {
	Path string `json:"path"`
}

// handleComposeImport 导入 Compose 项目目录（写回 config.yaml 并热加载）。
func (s *Service) handleComposeImport(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req composeImportRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path == "" {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, "项目目录不能为空")
		return
	}

	cfg := s.deps.State.Get()
	abs := cfg.Resolve(req.Path)
	if _, err := docker.ValidateProjectDir(abs); err != nil {
		writeErr(w, http.StatusBadRequest, CodeComposeInvalid, err.Error())
		return
	}
	if _, exists := cfg.FindComposeProject(abs); exists {
		writeErr(w, http.StatusConflict, CodeConflict, "该项目目录已导入")
		return
	}

	// 解析项目名（compose v2 支持时读取 name 字段）。
	name := docker.NormalizeProjectName(abs)
	if runner := s.deps.Compose(); runner != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		if n, err := runner.ProjectName(ctx, abs); err == nil && n != "" {
			name = n
		}
		cancel()
	}

	projects := append(append([]config.ComposeProject(nil), cfg.ComposeProjects...),
		config.ComposeProject{Name: name, Path: req.Path})
	if err := config.SetComposeProjects(cfg.Path, projects); err != nil {
		s.deps.Logger.Error("导入 Compose 项目写入配置失败", "user", sess.Username, "path", abs, "error", err)
		writeErr(w, http.StatusInternalServerError, CodeInternal, "保存失败: "+err.Error())
		return
	}
	if s.deps.ReloadConfig != nil {
		if err := s.deps.ReloadConfig(); err != nil {
			writeErr(w, http.StatusInternalServerError, CodeInternal, "配置已写入但热加载失败: "+err.Error())
			return
		}
	}

	s.deps.Logger.Info("导入 Compose 项目", "user", sess.Username, "target", abs, "name", name)
	writeJSON(w, http.StatusOK, map[string]any{
		"project": composeProjectItem{Name: name, Path: req.Path, AbsPath: abs, Valid: true},
	})
}

// handleComposeRemove 移除 Compose 项目（仅从配置移除，不触碰容器）。
func (s *Service) handleComposeRemove(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req composeImportRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Path = strings.TrimSpace(req.Path)

	cfg := s.deps.State.Get()
	target := filepath.Clean(cfg.Resolve(req.Path))
	projects := make([]config.ComposeProject, 0, len(cfg.ComposeProjects))
	found := false
	for _, p := range cfg.ComposeProjects {
		if filepath.Clean(cfg.ComposeProjectPath(p)) == target {
			found = true
			continue
		}
		projects = append(projects, p)
	}
	if !found {
		writeErr(w, http.StatusNotFound, CodeNotFound, "未找到该项目")
		return
	}

	if err := config.SetComposeProjects(cfg.Path, projects); err != nil {
		s.deps.Logger.Error("移除 Compose 项目写入配置失败", "user", sess.Username, "error", err)
		writeErr(w, http.StatusInternalServerError, CodeInternal, "保存失败: "+err.Error())
		return
	}
	if s.deps.ReloadConfig != nil {
		if err := s.deps.ReloadConfig(); err != nil {
			writeErr(w, http.StatusInternalServerError, CodeInternal, "配置已写入但热加载失败: "+err.Error())
			return
		}
	}
	s.deps.Logger.Info("移除 Compose 项目", "user", sess.Username, "target", target)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type composeActionRequest struct {
	Path            string `json:"path"`
	Action          string `json:"action"`
	DeleteOldImages bool   `json:"deleteOldImages"`
}

// handleComposeAction 执行 Compose 项目操作（异步任务）。
func (s *Service) handleComposeAction(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req composeActionRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Path = strings.TrimSpace(req.Path)

	cmds, err := docker.ActionArgs(req.Action)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	cfg := s.deps.State.Get()
	project, found := cfg.FindComposeProject(req.Path)
	if !found {
		writeErr(w, http.StatusNotFound, CodeNotFound, "未找到该项目，请先导入项目目录")
		return
	}
	abs := cfg.ComposeProjectPath(project)
	if _, err := docker.ValidateProjectDir(abs); err != nil {
		writeErr(w, http.StatusBadRequest, CodeComposeInvalid, err.Error())
		return
	}
	runner := s.deps.Compose()
	if runner == nil {
		writeErr(w, http.StatusServiceUnavailable, CodeDockerDown, "Docker Compose 不可用")
		return
	}
	client := s.deps.Docker()

	title := map[string]string{
		docker.ActionStart:   "启动",
		docker.ActionStop:    "停止",
		docker.ActionRestart: "重启",
		docker.ActionUpdate:  "更新",
		docker.ActionDown:    "关闭",
		docker.ActionPull:    "拉取",
	}[req.Action] + " Compose 项目 " + projectDisplay(project, abs)

	s.deps.Logger.Info("执行 Compose 操作",
		"user", sess.Username, "target", abs, "action", req.Action, "deleteOldImages", req.DeleteOldImages)

	job := s.deps.Jobs.Start("compose.action", title, func(jctx context.Context, out io.Writer) error {
		return s.runComposeAction(jctx, out, runner, client, abs, req.Action, cmds, req.DeleteOldImages)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID})
}

func projectDisplay(p config.ComposeProject, abs string) string {
	if p.Name != "" {
		return p.Name
	}
	return filepath.Base(abs)
}

// runComposeAction 执行 Compose 操作流水线。
// update：pull → down → up -d，完成后按需删除旧镜像。
func (s *Service) runComposeAction(
	ctx context.Context,
	out io.Writer,
	runner *docker.ComposeRunner,
	client *docker.Client,
	dir, action string,
	cmds [][]string,
	deleteOldImages bool,
) error {
	var oldImageIDs []string

	if action == docker.ActionUpdate {
		// 记录旧镜像 ID，供更新完成后清理。
		if refs, err := runner.ServiceImages(ctx, dir); err == nil {
			for _, ref := range refs {
				if client == nil {
					continue
				}
				inspect, err := client.InspectImage(ctx, ref)
				if err != nil {
					continue
				}
				oldImageIDs = append(oldImageIDs, inspect.ID)
				fmt.Fprintf(out, "记录旧镜像: %s (%s)\n", ref, docker.ShortID(inspect.ID))
			}
		}
	}

	for _, args := range cmds {
		if err := runner.Run(ctx, dir, out, args...); err != nil {
			return err
		}
	}

	if action == docker.ActionUpdate && deleteOldImages && client != nil && len(oldImageIDs) > 0 {
		fmt.Fprintln(out, "==> 清理旧镜像")
		for _, id := range oldImageIDs {
			if used, err := client.ImageUsage(ctx, id); err == nil && len(used) > 0 {
				fmt.Fprintf(out, "跳过 %s：仍被 %d 个容器使用\n", docker.ShortID(id), len(used))
				continue
			}
			if _, err := client.RemoveImage(ctx, id, false); err != nil {
				fmt.Fprintf(out, "跳过 %s：%v\n", docker.ShortID(id), err)
				continue
			}
			fmt.Fprintf(out, "已删除旧镜像 %s\n", docker.ShortID(id))
		}
	}

	fmt.Fprintln(out, "==> 操作完成")
	return nil
}
