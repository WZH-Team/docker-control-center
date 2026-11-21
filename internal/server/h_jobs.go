package server

import (
	"net/http"
	"strconv"
)

// handleJobsRecent 返回最近任务列表。
func (s *Service) handleJobsRecent(w http.ResponseWriter, r *http.Request, _ *Session) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.deps.Jobs.Recent(limit)})
}

// handleJobGet 返回任务详情（含输出）。
func (s *Service) handleJobGet(w http.ResponseWriter, r *http.Request, _ *Session) {
	id := r.PathValue("id")
	job, ok := s.deps.Jobs.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, CodeJobNotFound, "任务不存在或已过期")
		return
	}
	writeJSON(w, http.StatusOK, job.Snapshot())
}

// handleJobCancel 取消任务。
func (s *Service) handleJobCancel(w http.ResponseWriter, r *http.Request, sess *Session) {
	id := r.PathValue("id")
	job, ok := s.deps.Jobs.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, CodeJobNotFound, "任务不存在或已过期")
		return
	}
	job.Cancel()
	s.deps.Logger.Info("请求取消任务", "user", sess.Username, "job", id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
