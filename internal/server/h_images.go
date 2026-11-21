package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"dcc/internal/docker"
)

// containerRef 容器引用简述。
type containerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// imageItem 镜像列表条目。
type imageItem struct {
	ID          string        `json:"id"`
	IDShort     string        `json:"idShort"`
	RepoTags    []string      `json:"repoTags"`
	RepoDigests []string      `json:"repoDigests"`
	Created     string        `json:"created"`
	Size        int64         `json:"size"`
	IsDangling  bool          `json:"isDangling"`
	InUseBy     []containerRef `json:"inUseBy"`
}

// buildUsageIndex 构建镜像 ID → 使用中容器 的索引。
func buildUsageIndex(containers []docker.ContainerSummary) map[string][]containerRef {
	usage := make(map[string][]containerRef)
	for _, ctr := range containers {
		key := docker.TrimImageID(ctr.ImageID)
		usage[key] = append(usage[key], containerRef{
			ID:   ctr.ID,
			Name: docker.ContainerName(ctr.Names, docker.ShortID(ctr.ID)),
		})
	}
	return usage
}

// handleImagesList 列出全部镜像（含使用该镜像的容器）。
func (s *Service) handleImagesList(w http.ResponseWriter, r *http.Request, _ *Session) {
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	images, err := client.ListImages(r.Context())
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	containers, err := client.ListContainers(r.Context(), true)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	usage := buildUsageIndex(containers)

	items := make([]imageItem, 0, len(images))
	for _, img := range images {
		items = append(items, imageItem{
			ID:          img.ID,
			IDShort:     docker.ShortID(img.ID),
			RepoTags:    nonNil(img.RepoTags),
			RepoDigests: nonNil(img.RepoDigests),
			Created:     formatUnix(img.Created),
			Size:        img.Size,
			IsDangling:  len(img.RepoTags) == 0,
			InUseBy:     usage[docker.TrimImageID(img.ID)],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": items})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// handleImageDetail 返回镜像详情。
func (s *Service) handleImageDetail(w http.ResponseWriter, r *http.Request, _ *Session) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if err := docker.ValidateDockerID(ref); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	inspect, err := client.InspectImage(r.Context(), ref)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	containers, err := client.ListContainers(r.Context(), true)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	usage := buildUsageIndex(containers)

	detail := map[string]any{
		"id":           inspect.ID,
		"idShort":      docker.ShortID(inspect.ID),
		"repoTags":     nonNil(inspect.RepoTags),
		"repoDigests":  nonNil(inspect.RepoDigests),
		"created":      inspect.Created,
		"size":         inspect.Size,
		"architecture": inspect.Architecture,
		"os":           inspect.Os,
		"author":       inspect.Author,
		"comment":      inspect.Comment,
		"layers":       len(inspect.RootFS.Layers),
		"inUseBy":      usage[docker.TrimImageID(inspect.ID)],
	}
	writeJSON(w, http.StatusOK, detail)
}

type imagesDeleteRequest struct {
	Refs  []string `json:"refs"`
	Force bool     `json:"force"`
}

type itemResult struct {
	Ref    string `json:"ref"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// handleImagesDelete 批量删除镜像（逐个返回结果）。
func (s *Service) handleImagesDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req imagesDeleteRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	if len(req.Refs) == 0 || len(req.Refs) > 100 {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, "请选择 1-100 个镜像")
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}

	// 未强制删除时，先检查镜像占用，给出明确错误。
	if !req.Force {
		containers, err := client.ListContainers(r.Context(), true)
		if err != nil {
			writeDockerErr(w, err)
			return
		}
		usage := buildUsageIndex(containers)
		for _, ref := range req.Refs {
			if err := docker.ValidateDockerID(ref); err != nil {
				writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
				return
			}
			if inspect, err := client.InspectImage(r.Context(), ref); err == nil {
				if used := usage[docker.TrimImageID(inspect.ID)]; len(used) > 0 {
					writeErr(w, http.StatusConflict, CodeImageInUse,
						fmt.Sprintf("镜像 %s 正被 %d 个容器使用，请先处理这些容器或使用强制删除", ref, len(used)))
					return
				}
			}
		}
	}

	results := make([]itemResult, 0, len(req.Refs))
	for _, ref := range req.Refs {
		if err := docker.ValidateDockerID(ref); err != nil {
			results = append(results, itemResult{Ref: ref, OK: false, Error: err.Error()})
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		_, err := client.RemoveImage(ctx, ref, req.Force)
		cancel()
		if err != nil {
			results = append(results, itemResult{Ref: ref, OK: false, Error: err.Error()})
			s.deps.Logger.Warn("删除镜像失败", "user", sess.Username, "target", ref, "error", err)
			continue
		}
		results = append(results, itemResult{Ref: ref, OK: true})
		s.deps.Logger.Info("删除镜像成功", "user", sess.Username, "target", ref)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

type imagePullRequest struct {
	Ref string `json:"ref"`
}

// handleImagePull 拉取新镜像（异步任务）。
func (s *Service) handleImagePull(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req imagePullRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Ref = strings.TrimSpace(req.Ref)
	if _, _, err := docker.ParseImageRef(req.Ref); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}

	s.deps.Logger.Info("开始拉取镜像", "user", sess.Username, "target", req.Ref)
	job := s.deps.Jobs.Start("image.pull", "拉取镜像 "+req.Ref, func(ctx context.Context, out io.Writer) error {
		return client.PullImage(ctx, req.Ref, out)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID})
}

// handleImageUpdate 更新镜像：仅在无容器使用时拉取新镜像，随后删除旧镜像。
func (s *Service) handleImageUpdate(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req imagePullRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	req.Ref = strings.TrimSpace(req.Ref)
	if _, _, err := docker.ParseImageRef(req.Ref); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}

	// 更新前置检查：必须确保无容器使用该镜像（安全策略）。
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	inspect, err := client.InspectImage(ctx, req.Ref)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	containers, err := client.ListContainers(ctx, true)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	usage := buildUsageIndex(containers)
	if used := usage[docker.TrimImageID(inspect.ID)]; len(used) > 0 {
		names := make([]string, 0, len(used))
		for _, u := range used {
			names = append(names, u.Name)
		}
		sort.Strings(names)
		writeErr(w, http.StatusConflict, CodeImageInUse,
			"镜像正被容器使用，无法更新："+strings.Join(names, ", "))
		return
	}
	oldID := inspect.ID

	s.deps.Logger.Info("开始更新镜像", "user", sess.Username, "target", req.Ref)
	job := s.deps.Jobs.Start("image.update", "更新镜像 "+req.Ref, func(jctx context.Context, out io.Writer) error {
		return s.runImageUpdate(jctx, out, client, req.Ref, oldID)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID})
}

// runImageUpdate 执行镜像更新流水线（拉取 → 校验 → 删除旧镜像）。
func (s *Service) runImageUpdate(ctx context.Context, out io.Writer, client *docker.Client, ref, oldID string) error {
	fmt.Fprintf(out, "==> 拉取镜像 %s\n", ref)
	if err := client.PullImage(ctx, ref, out); err != nil {
		return err
	}

	// 拉取后解析新镜像 ID。
	inspect, err := client.InspectImage(ctx, ref)
	if err != nil {
		return fmt.Errorf("解析新镜像失败: %w", err)
	}
	newID := inspect.ID
	if docker.TrimImageID(newID) == docker.TrimImageID(oldID) {
		fmt.Fprintln(out, "镜像内容无变化（已是最新），无需清理旧镜像。")
		return nil
	}

	fmt.Fprintf(out, "==> 删除旧镜像 %s\n", docker.ShortID(oldID))
	if used, err := client.ImageUsage(ctx, oldID); err == nil && len(used) > 0 {
		fmt.Fprintf(out, "旧镜像仍被 %d 个容器使用，跳过删除。\n", len(used))
		return nil
	}
	if _, err := client.RemoveImage(ctx, oldID, false); err != nil {
		// 旧镜像可能仍被其他标签引用，属正常情况。
		fmt.Fprintf(out, "旧镜像删除跳过: %v\n", err)
		return nil
	}
	fmt.Fprintln(out, "旧镜像已删除。")
	return nil
}
