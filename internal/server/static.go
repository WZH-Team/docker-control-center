package server

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// spaHandler 提供内嵌前端静态资源，并对前端路由做 index.html 回退。
type spaHandler struct {
	static fs.FS
	index  []byte
	mtime  time.Time
}

// newSPAHandler 构建静态资源处理器（root 为 web/static 目录）。
func newSPAHandler(static fs.FS) (*spaHandler, error) {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	return &spaHandler{static: sub, index: index, mtime: time.Now()}, nil
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErr(w, http.StatusMethodNotAllowed, CodeBadRequest, "不支持的请求方法")
		return
	}

	upath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	// API 未匹配路由统一返回 JSON 404。
	if strings.HasPrefix(upath, "api/") || upath == "api" {
		writeErr(w, http.StatusNotFound, CodeNotFound, "接口不存在")
		return
	}

	if upath == "" || upath == "." {
		h.serveIndex(w)
		return
	}

	f, err := h.static.Open(upath)
	if err != nil {
		// 未知路径回退到 SPA 入口（前端路由自行处理）。
		h.serveIndex(w)
		return
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		h.serveIndex(w)
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		h.serveIndex(w)
		return
	}

	ctype := mime.TypeByExtension(path.Ext(upath))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	// 管理面板资源体量小，统一不缓存，保证升级二进制后立即生效。
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, upath, st.ModTime(), rs)
}

func (h *spaHandler) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.index)
}
