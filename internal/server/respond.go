package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"dcc/internal/docker"
)

// 统一 API 错误码（前端 i18n 依据 code 映射本地化文案，message 为兜底）。
const (
	CodeBadRequest      = "err.bad_request"
	CodeUnauthorized    = "err.unauthorized"
	CodeForbidden       = "err.forbidden"
	CodeCSRF            = "err.csrf"
	CodeRateLimited     = "err.rate_limited"
	CodeNotFound        = "err.not_found"
	CodeConflict        = "err.conflict"
	CodeInternal        = "err.internal"
	CodeDockerDown      = "err.docker.unavailable"
	CodeDockerMissing   = "err.docker.not_installed"
	CodeImageInUse      = "err.image.in_use"
	CodeComposeInvalid  = "err.compose.invalid_dir"
	CodeJobNotFound     = "err.job.not_found"
	CodeUnsupportedOS   = "err.unsupported_platform"
	CodeConfigInvalid   = "err.config.invalid"
	CodeLoginFailed     = "err.login_failed"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	OK    bool      `json:"ok"`
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
}

// writeJSON 输出成功响应。
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{OK: true, Data: data})
}

// writeErr 输出错误响应。
func writeErr(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{OK: false, Error: &apiError{Code: code, Message: message}})
}

// decodeBody 解析 JSON 请求体（限制大小，防止内存滥用）。
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("请求体不是合法的 JSON")
	}
	return nil
}

// writeDockerErr 将 Docker 客户端错误映射为 API 错误响应。
func writeDockerErr(w http.ResponseWriter, err error) {
	var apiErr *docker.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusNotFound:
			writeErr(w, http.StatusNotFound, CodeNotFound, apiErr.Message)
		case apiErr.StatusCode == http.StatusConflict:
			writeErr(w, http.StatusConflict, CodeConflict, apiErr.Message)
		case apiErr.StatusCode == http.StatusBadRequest:
			writeErr(w, http.StatusBadRequest, CodeBadRequest, apiErr.Message)
		default:
			writeErr(w, http.StatusBadGateway, CodeDockerDown, apiErr.Message)
		}
		return
	}
	writeErr(w, http.StatusBadGateway, CodeDockerDown, err.Error())
}

// formatUnix 将 Unix 秒级时间戳格式化为 RFC3339。
func formatUnix(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}
