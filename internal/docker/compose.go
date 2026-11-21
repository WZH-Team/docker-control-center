package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ComposeRunner 执行 docker compose 命令（兼容 v2 插件与 v1 docker-compose）。
type ComposeRunner struct {
	dockerHost string

	mu       sync.Mutex
	checked  bool
	v2       bool // true: docker compose；false: docker-compose
	availErr error
}

// NewComposeRunner 创建 Compose 执行器。dockerHost 非空时注入 DOCKER_HOST 环境变量。
func NewComposeRunner(dockerHost string) *ComposeRunner {
	return &ComposeRunner{dockerHost: strings.TrimSpace(dockerHost)}
}

// baseCommand 返回 compose 命令前缀。
func (r *ComposeRunner) baseCommand() []string {
	if r.v2 {
		return []string{"docker", "compose"}
	}
	return []string{"docker-compose"}
}

// detect 探测可用的 compose 命令（结果缓存）。
func (r *ComposeRunner) detect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.checked {
		return r.availErr
	}
	r.checked = true

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := exec.CommandContext(ctx, "docker", "compose", "version", "--short").Run(); err == nil {
		r.v2 = true
		r.availErr = nil
		return nil
	}
	if err := exec.CommandContext(ctx, "docker-compose", "version", "--short").Run(); err == nil {
		r.v2 = false
		r.availErr = nil
		return nil
	}
	r.availErr = errors.New("未检测到 docker compose（v2 插件）或 docker-compose（v1），请先安装 Docker Compose")
	return r.availErr
}

// ResetDetection 清除探测缓存（Docker 安装完成后调用）。
func (r *ComposeRunner) ResetDetection() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checked = false
	r.availErr = nil
}

// Available 返回 compose 是否可用。
func (r *ComposeRunner) Available(ctx context.Context) bool {
	return r.detect(ctx) == nil
}

// commandEnv 构造子进程环境变量。
func (r *ComposeRunner) commandEnv() []string {
	env := os.Environ()
	if r.dockerHost != "" {
		env = append(env, "DOCKER_HOST="+r.dockerHost)
	}
	// 关闭交互式彩色输出，保证日志可读。
	env = append(env, "COMPOSE_ANSI=never", "NO_COLOR=1")
	return env
}

// ValidateProjectDir 校验 Compose 项目目录有效性，返回发现的 compose 文件名。
func ValidateProjectDir(dir string) (string, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("项目目录不存在或不是目录: %s", dir)
	}
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() {
			return name, nil
		}
	}
	return "", fmt.Errorf("目录中未找到 compose.yaml / compose.yml / docker-compose.yml / docker-compose.yaml: %s", dir)
}

// Run 在项目目录下执行 compose 子命令，输出写入 out。
func (r *ComposeRunner) Run(ctx context.Context, dir string, out io.Writer, args ...string) error {
	if err := r.detect(ctx); err != nil {
		return err
	}
	if _, err := ValidateProjectDir(dir); err != nil {
		return err
	}

	full := append(r.baseCommand(), "--project-directory", dir)
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, full[0], full[1:]...)
	cmd.Env = r.commandEnv()
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out

	if out != nil {
		fmt.Fprintf(out, "$ %s\n", strings.Join(full, " "))
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("命令已取消: %w", ctx.Err())
		}
		return fmt.Errorf("命令执行失败: %w", err)
	}
	return nil
}

// composeFileNames 各 compose 文件名（用于 compose config 解析）。

var projectNameSanitizer = regexp.MustCompile(`[^a-z0-9_-]+`)

// NormalizeProjectName 将目录名规范化为 Compose 项目名。
func NormalizeProjectName(dir string) string {
	base := strings.ToLower(filepath.Base(filepath.Clean(dir)))
	name := projectNameSanitizer.ReplaceAllString(base, "-")
	name = strings.Trim(name, "-_")
	if name == "" {
		name = "project"
	}
	return name
}

// ProjectName 解析 Compose 项目名（优先 `compose config --format json`，失败则按目录名推导）。
func (r *ComposeRunner) ProjectName(ctx context.Context, dir string) (string, error) {
	if err := r.detect(ctx); err != nil {
		return "", err
	}
	name := NormalizeProjectName(dir)

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	full := append(r.baseCommand(), "--project-directory", dir, "config", "--format", "json")
	cmd := exec.CommandContext(ctx, full[0], full[1:]...)
	cmd.Env = r.commandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// 旧版本 compose 不支持 --format json，回退目录名。
		return name, nil
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err == nil && payload.Name != "" {
		return payload.Name, nil
	}
	return name, nil
}

// ServiceImages 返回项目定义的镜像引用列表（`compose config --images`）。
func (r *ComposeRunner) ServiceImages(ctx context.Context, dir string) ([]string, error) {
	if err := r.detect(ctx); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	full := append(r.baseCommand(), "--project-directory", dir, "config", "--images")
	cmd := exec.CommandContext(ctx, full[0], full[1:]...)
	cmd.Env = r.commandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("读取项目镜像列表失败: %s", strings.TrimSpace(stderr.String()))
	}

	var refs []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			refs = append(refs, line)
		}
	}
	return refs, nil
}

// ComposeActions 项目支持的操作。
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionUpdate  = "update"
	ActionDown    = "down"
	ActionPull    = "pull"
)

// ActionArgs 将操作映射为 compose 参数序列。
// update 操作返回多组命令（pull -> down -> up -d），由调用方按序执行。
func ActionArgs(action string) ([][]string, error) {
	switch action {
	case ActionStart:
		return [][]string{{"up", "-d"}}, nil
	case ActionStop:
		return [][]string{{"stop"}}, nil
	case ActionRestart:
		return [][]string{{"restart"}}, nil
	case ActionDown:
		return [][]string{{"down"}}, nil
	case ActionPull:
		return [][]string{{"pull"}}, nil
	case ActionUpdate:
		return [][]string{{"pull"}, {"down"}, {"up", "-d"}}, nil
	default:
		return nil, fmt.Errorf("不支持的操作: %q", action)
	}
}

// IsLinux 判断当前是否为 Linux 环境（Docker 安装脚本仅支持 Linux）。
func IsLinux() bool { return runtime.GOOS == "linux" }

// InstallDocker 在 Debian/Ubuntu 系机器上执行官方安装脚本。
// 输出以流式方式写入 out；命令固定为 `curl https://get.docker.com | bash`，无任何外部输入拼接。
func InstallDocker(ctx context.Context, out io.Writer) error {
	if !IsLinux() {
		return errors.New("自动安装仅支持 Debian/Ubuntu 等 Linux 系统")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return errors.New("未找到 bash，无法执行安装脚本")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		return errors.New("未找到 curl，无法下载安装脚本")
	}

	const script = "curl -fsSL https://get.docker.com | bash"
	fmt.Fprintf(out, "$ %s\n", script)

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Env = os.Environ()
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("安装已取消: %w", ctx.Err())
		}
		return fmt.Errorf("Docker 安装脚本执行失败: %w", err)
	}
	fmt.Fprintln(out, "Docker 安装脚本执行完成。")
	return nil
}
