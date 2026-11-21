package server

import (
	"net/http"

	"dcc/web"
)

// authed 包装只读（GET）接口的登录校验。
func (s *Service) authed(fn authedFunc) http.HandlerFunc {
	return s.requireAuth(fn)
}

// authedMutate 包装写操作接口：登录校验 + CSRF 校验。
func (s *Service) authedMutate(fn authedFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request, sess *Session) {
		if mutatingMethod(r.Method) && !checkCSRF(r, sess) {
			writeErr(w, http.StatusForbidden, CodeCSRF, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		fn(w, r, sess)
	})
}

// routes 组装全部路由。
func (s *Service) routes() http.Handler {
	mux := http.NewServeMux()

	// 会话与系统。
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.authedMutate(s.handleLogout))
	mux.HandleFunc("GET /api/status", s.authed(s.handleStatus))
	mux.HandleFunc("POST /api/docker/install", s.authedMutate(s.handleDockerInstall))
	mux.HandleFunc("GET /api/system/logs", s.authed(s.handleLogsTail))

	// 配置。
	mux.HandleFunc("GET /api/config/view", s.authed(s.handleConfigView))
	mux.HandleFunc("POST /api/config/reload", s.authedMutate(s.handleConfigReload))
	mux.HandleFunc("POST /api/config/auth", s.authedMutate(s.handleConfigAuth))

	// 容器。
	mux.HandleFunc("GET /api/containers", s.authed(s.handleContainersList))
	mux.HandleFunc("GET /api/containers/{id}", s.authed(s.handleContainerDetail))
	mux.HandleFunc("POST /api/containers/{id}/start", s.authedMutate(s.containerAction("start")))
	mux.HandleFunc("POST /api/containers/{id}/stop", s.authedMutate(s.containerAction("stop")))
	mux.HandleFunc("POST /api/containers/{id}/restart", s.authedMutate(s.containerAction("restart")))
	mux.HandleFunc("POST /api/containers/{id}/update", s.authedMutate(s.handleContainerUpdate))
	mux.HandleFunc("DELETE /api/containers/{id}", s.authedMutate(s.handleContainerDelete))

	// 镜像。
	mux.HandleFunc("GET /api/images", s.authed(s.handleImagesList))
	mux.HandleFunc("GET /api/images/detail", s.authed(s.handleImageDetail))
	mux.HandleFunc("DELETE /api/images", s.authedMutate(s.handleImagesDelete))
	mux.HandleFunc("POST /api/images/pull", s.authedMutate(s.handleImagePull))
	mux.HandleFunc("POST /api/images/update", s.authedMutate(s.handleImageUpdate))

	// 存储卷。
	mux.HandleFunc("GET /api/volumes", s.authed(s.handleVolumesList))
	mux.HandleFunc("GET /api/volumes/detail", s.authed(s.handleVolumeDetail))
	mux.HandleFunc("DELETE /api/volumes", s.authedMutate(s.handleVolumesDelete))

	// Compose 项目。
	mux.HandleFunc("GET /api/compose/projects", s.authed(s.handleComposeProjects))
	mux.HandleFunc("POST /api/compose/projects", s.authedMutate(s.handleComposeImport))
	mux.HandleFunc("DELETE /api/compose/projects", s.authedMutate(s.handleComposeRemove))
	mux.HandleFunc("POST /api/compose/action", s.authedMutate(s.handleComposeAction))

	// 任务。
	mux.HandleFunc("GET /api/jobs", s.authed(s.handleJobsRecent))
	mux.HandleFunc("GET /api/jobs/{id}", s.authed(s.handleJobGet))
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.authedMutate(s.handleJobCancel))

	// 静态资源与 SPA 回退。
	spa, err := newSPAHandler(web.Static)
	if err != nil {
		panic("初始化前端资源失败: " + err.Error())
	}
	mux.Handle("/", spa)

	return mux
}
