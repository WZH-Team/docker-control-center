package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"dcc/internal/docker"
)

// volumeItem 存储卷列表条目。
type volumeItem struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	CreatedAt  string            `json:"createdAt"`
	Labels     map[string]string `json:"labels"`
	Scope      string            `json:"scope"`
	Size       int64             `json:"size,omitempty"`
	RefCount   int64             `json:"refCount,omitempty"`
}

func toVolumeItem(v *docker.Volume) volumeItem {
	item := volumeItem{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		CreatedAt:  v.CreatedAt,
		Labels:     v.Labels,
		Scope:      v.Scope,
	}
	if v.Labels == nil {
		item.Labels = map[string]string{}
	}
	if v.UsageData != nil {
		item.Size = v.UsageData.Size
		item.RefCount = v.UsageData.RefCount
	}
	return item
}

// handleVolumesList 列出全部存储卷。
func (s *Service) handleVolumesList(w http.ResponseWriter, r *http.Request, _ *Session) {
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	volumes, err := client.ListVolumes(r.Context())
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	items := make([]volumeItem, 0, len(volumes))
	for _, v := range volumes {
		items = append(items, toVolumeItem(v))
	}
	writeJSON(w, http.StatusOK, map[string]any{"volumes": items})
}

// handleVolumeDetail 返回存储卷详情。
func (s *Service) handleVolumeDetail(w http.ResponseWriter, r *http.Request, _ *Session) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if err := docker.ValidateDockerID(name); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}
	v, err := client.InspectVolume(r.Context(), name)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	detail := toVolumeItem(v)
	writeJSON(w, http.StatusOK, map[string]any{
		"volume":   detail,
		"options":  v.Options,
		"status":   v.Status,
		"warnings": []string{},
	})
}

type volumesDeleteRequest struct {
	Names []string `json:"names"`
	Force bool     `json:"force"`
}

// handleVolumesDelete 批量删除存储卷（逐个返回结果）。
func (s *Service) handleVolumesDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	var req volumesDeleteRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	if len(req.Names) == 0 || len(req.Names) > 100 {
		writeErr(w, http.StatusBadRequest, CodeBadRequest, "请选择 1-100 个存储卷")
		return
	}
	client, ok := s.dockerClient(w)
	if !ok {
		return
	}

	results := make([]itemResult, 0, len(req.Names))
	for _, name := range req.Names {
		if err := docker.ValidateDockerID(name); err != nil {
			results = append(results, itemResult{Ref: name, OK: false, Error: err.Error()})
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err := client.RemoveVolume(ctx, name, req.Force)
		cancel()
		if err != nil {
			results = append(results, itemResult{Ref: name, OK: false, Error: err.Error()})
			s.deps.Logger.Warn("删除存储卷失败", "user", sess.Username, "target", name, "error", err)
			continue
		}
		results = append(results, itemResult{Ref: name, OK: true})
		s.deps.Logger.Info("删除存储卷成功", "user", sess.Username, "target", name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
