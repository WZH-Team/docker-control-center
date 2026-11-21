// Package jobs 提供进程内异步任务（长耗时操作）管理：
// 拉取镜像、Compose 项目操作、Docker 安装等都以任务形式执行，
// 前端通过轮询获取状态与输出，避免 HTTP 长连接超时问题。
package jobs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Status 任务状态。
type Status string

const (
	StatusQueued   Status = "queued"
	StatusRunning  Status = "running"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

// 任务输出上限与任务保留策略。
const (
	maxOutputBytes = 256 * 1024
	maxJobs        = 100
	retention      = 30 * time.Minute
)

// Info 任务快照（可直接序列化为 JSON）。
type Info struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Status     Status     `json:"status"`
	Output     string     `json:"output"`
	OutputFull bool       `json:"outputTruncated"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// boundedBuffer 限量输出缓冲，超出上限后丢弃后续内容并做标记。
type boundedBuffer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	max     int
	dropped bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() >= b.max {
		b.dropped = true
		return len(p), nil
	}
	remain := b.max - b.buf.Len()
	if len(p) > remain {
		b.buf.Write(p[:remain])
		b.dropped = true
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) snapshot() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String(), b.dropped
}

// Job 单个异步任务。
type Job struct {
	ID    string
	Type  string
	Title string

	mu         sync.Mutex
	status     Status
	output     *boundedBuffer
	err        error
	createdAt  time.Time
	startedAt  time.Time
	finishedAt time.Time
	cancel     context.CancelFunc
}

// Fn 任务执行函数，输出写入 w。
type Fn func(ctx context.Context, w io.Writer) error

// Manager 任务管理器。
type Manager struct {
	mu     sync.Mutex
	jobs   map[string]*Job
	order  []string
	logger *slog.Logger
}

// NewManager 创建任务管理器。
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		jobs:   make(map[string]*Job),
		logger: logger,
	}
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}

// Start 启动一个异步任务并立即返回任务句柄。
func (m *Manager) Start(kind, title string, fn Fn) *Job {
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{
		ID:        newID(),
		Type:      kind,
		Title:     title,
		status:    StatusRunning,
		output:    &boundedBuffer{max: maxOutputBytes},
		createdAt: time.Now(),
		startedAt: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.jobs[j.ID] = j
	m.order = append(m.order, j.ID)
	m.pruneLocked()
	m.mu.Unlock()

	go func() {
		defer cancel()
		err := fn(ctx, j.output)
		j.mu.Lock()
		j.finishedAt = time.Now()
		switch {
		case err == nil:
			j.status = StatusSuccess
		case errors.Is(err, context.Canceled) || ctx.Err() != nil:
			j.status = StatusCanceled
			j.err = errors.New("任务已取消")
		default:
			j.status = StatusFailed
			j.err = err
		}
		j.mu.Unlock()

		if m.logger != nil {
			if err != nil {
				m.logger.Warn("任务结束", "job", j.ID, "type", kind, "title", title, "status", j.Status(), "error", err)
			} else {
				m.logger.Info("任务结束", "job", j.ID, "type", kind, "title", title, "status", j.Status())
			}
		}
	}()
	return j
}

// pruneLocked 清理过期任务（调用方持有锁）。
func (m *Manager) pruneLocked() {
	now := time.Now()
	// 先按时间清理已完成任务。
	keep := m.order[:0]
	for _, id := range m.order {
		j, ok := m.jobs[id]
		if !ok {
			continue
		}
		j.mu.Lock()
		finished := j.status == StatusSuccess || j.status == StatusFailed || j.status == StatusCanceled
		doneAt := j.finishedAt
		j.mu.Unlock()
		if finished && !doneAt.IsZero() && now.Sub(doneAt) > retention {
			delete(m.jobs, id)
			continue
		}
		keep = append(keep, id)
	}
	m.order = keep

	// 数量兜底：淘汰最早的已结束任务。
	for len(m.order) > maxJobs {
		removed := false
		for i, id := range m.order {
			j := m.jobs[id]
			j.mu.Lock()
			finished := j.status == StatusSuccess || j.status == StatusFailed || j.status == StatusCanceled
			j.mu.Unlock()
			if finished {
				delete(m.jobs, id)
				m.order = append(m.order[:i], m.order[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			break
		}
	}
}

// Get 按 ID 获取任务。
func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

// Recent 返回最近的任务（按启动时间倒序）。
func (m *Manager) Recent(limit int) []Info {
	m.mu.Lock()
	ids := append([]string(nil), m.order...)
	m.mu.Unlock()

	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}
	out := make([]Info, 0, limit)
	for i := len(ids) - 1; i >= 0 && len(out) < limit; i-- {
		if j, ok := m.Get(ids[i]); ok {
			out = append(out, j.Snapshot())
		}
	}
	return out
}

// Status 返回任务状态。
func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// Cancel 请求取消任务。
func (j *Job) Cancel() {
	j.mu.Lock()
	cancel := j.cancel
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Snapshot 返回任务快照。
func (j *Job) Snapshot() Info {
	j.mu.Lock()
	defer j.mu.Unlock()

	info := Info{
		ID:        j.ID,
		Type:      j.Type,
		Title:     j.Title,
		Status:    j.status,
		CreatedAt: j.createdAt,
	}
	info.Output, info.OutputFull = j.output.snapshot()
	if j.err != nil {
		info.Error = j.err.Error()
	}
	if !j.startedAt.IsZero() {
		t := j.startedAt
		info.StartedAt = &t
	}
	if !j.finishedAt.IsZero() {
		t := j.finishedAt
		info.FinishedAt = &t
	}
	return info
}
