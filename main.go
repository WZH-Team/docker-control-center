// Docker Control Center (DCC) —— 轻量、单文件的 Docker Web 管理面板。
//
// 启动流程：
//  1. 定位 config.yaml（默认在程序目录），不存在则自动生成（含 16 位随机密码）；
//  2. 初始化日志、Docker 客户端、任务管理器与 Web 服务（HTTP/HTTPS 按配置）；
//  3. 启动配置热重载监听与优雅退出处理。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"dcc/internal/config"
	"dcc/internal/console"
	"dcc/internal/docker"
	"dcc/internal/jobs"
	"dcc/internal/logging"
	"dcc/internal/server"
)

const appVersion = "0.3.5"

// app 运行期组件容器。
type app struct {
	state        *server.State
	logs         *logging.Manager
	jobs         *jobs.Manager
	srv          *server.Service
	dockerClient atomic.Pointer[docker.Client]
	composeRun   atomic.Pointer[docker.ComposeRunner]
	configPath   string
}

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认：<程序目录>/config.yaml）")
	showVersion := flag.Bool("version", false, "显示版本号并退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("DCC (Docker Control Center) v" + appVersion)
		return
	}

	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "[DCC] 启动失败:", err)
		os.Exit(1)
	}
}

// resolveConfigPath 计算配置文件绝对路径。
func resolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位程序目录: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Join(filepath.Dir(exe), "config.yaml"), nil
}

func run(explicitConfig string) error {
	configPath, err := resolveConfigPath(explicitConfig)
	if err != nil {
		return err
	}

	a := &app{configPath: configPath}

	// 控制台输出目标：交互式终端前台运行时输出到 stdout，
	// 以 & / nohup / systemd 等方式后台运行时静默（日志仍写入日志文件）。
	consoleOut := console.Writer()

	// 1. 配置：不存在则首次生成（含随机密码）。
	var firstRunPassword string
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		_, password, err := config.Generate(configPath)
		if err != nil {
			return fmt.Errorf("生成配置文件失败: %w", err)
		}
		firstRunPassword = password
		printFirstRunInfo(consoleOut, configPath, password)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	// 2. 日志。
	logs, err := logging.New(cfg.Logging, cfg.BaseDir, consoleOut)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[DCC] 警告: 日志初始化失败，仅输出到控制台:", err)
	}
	a.logs = logs
	logger := logs.Logger()

	if firstRunPassword != "" {
		// 按需求：首次生成的密码必须记录进日志（不受 logging.level 过滤影响）。
		logs.Important("首次启动已生成登录密码",
			"config", configPath,
			"username", cfg.Auth.Username,
			"password", firstRunPassword)
	}
	logger.Info("DCC 启动中", "version", appVersion, "config", configPath, "go", runtime.GOOS)

	// 3. 任务管理器与运行组件。
	a.jobs = jobs.NewManager(logger)
	a.state = server.NewState(cfg)
	a.refreshClients(cfg)

	// 4. Web 服务。
	a.srv = server.New(&server.Deps{
		State:        a.state,
		Logger:       logger,
		Jobs:         a.jobs,
		Docker:       func() *docker.Client { return a.dockerClient.Load() },
		Compose:      func() *docker.ComposeRunner { return a.composeRun.Load() },
		ReloadConfig: a.reloadConfig,
		Version:      appVersion,
		Started:      time.Now(),
	})
	if err := a.srv.Start(); err != nil {
		return err
	}
	logs.StartupMessage("[DCC] Docker Control Center v%s 已启动：访问 %s", appVersion, a.srv.ListenDesc())
	logs.StartupMessage("[DCC] 登录用户名: %s", cfg.Auth.Username)
	if firstRunPassword != "" {
		logs.StartupMessage("[DCC] 登录密码: %s （请立即登录并妥善保存）", firstRunPassword)
	}

	// 5. 配置热重载。
	watcher := config.NewWatcher(configPath, logger,
		func(newCfg *config.Config) {
			if err := a.applyConfig(newCfg); err != nil {
				logger.Error("应用热重载配置失败，保持现有配置", "error", err)
			}
		},
		func(err error) {
			logger.Error("配置热重载失败，保持现有配置", "error", err)
		})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx)

	// 6. 优雅退出。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh
	logs.StartupMessage("[DCC] 收到信号 %v，正在优雅退出...", sig)
	logger.Info("收到退出信号", "signal", sig.String())

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("关闭 Web 服务时出错", "error", err)
	}
	logger.Info("DCC 已退出")
	_ = a.logs.Close()
	return nil
}

// refreshClients 依据配置创建/重建 Docker 客户端与 Compose 执行器。
func (a *app) refreshClients(cfg *config.Config) {
	logger := a.logs.Logger()

	client, err := docker.NewClient(cfg.Docker.Host)
	if err != nil {
		logger.Warn("Docker 客户端初始化失败", "error", err,
			"hint", "可在 config.yaml 的 docker.host 配置地址，或安装 Docker 后刷新页面")
		a.dockerClient.Store(nil)
	} else {
		a.dockerClient.Store(client)
	}

	host := cfg.Docker.Host
	if client != nil {
		host = client.Host()
	}
	a.composeRun.Store(docker.NewComposeRunner(host))
}

// applyConfig 应用新配置（监听、日志、Docker 连接等热更新）。
func (a *app) applyConfig(newCfg *config.Config) error {
	old := a.state.Get()

	if err := a.logs.Apply(newCfg.Logging, newCfg.BaseDir); err != nil {
		a.logs.Logger().Warn("日志配置应用失败", "error", err)
	}
	a.state.Set(newCfg)

	if old.Docker.Host != newCfg.Docker.Host {
		a.refreshClients(newCfg)
	}

	if err := a.srv.ApplyConfig(old, newCfg); err != nil {
		return err
	}
	a.logs.Logger().Info("配置已热重载",
		"port", newCfg.Server.Port,
		"ssl", newCfg.SSL.Enabled,
		"logging", newCfg.Logging.Enabled,
		"user", newCfg.Auth.Username)
	return nil
}

// reloadConfig 重新读取并应用配置文件（供 API 调用）。
func (a *app) reloadConfig() error {
	newCfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	return a.applyConfig(newCfg)
}

// printFirstRunInfo 首次启动信息输出到控制台（后台静默运行时为 io.Discard）。
func printFirstRunInfo(w io.Writer, configPath, password string) {
	fmt.Fprintln(w, "============================================================")
	fmt.Fprintln(w, " Docker Control Center (DCC) 首次启动")
	fmt.Fprintln(w, " 已自动生成配置文件:", configPath)
	fmt.Fprintln(w, " 登录用户名: admin")
	fmt.Fprintln(w, " 登录密码:", password)
	fmt.Fprintln(w, " 请立即登录并妥善保存密码（已同步写入 config.yaml 与日志）")
	fmt.Fprintln(w, "============================================================")
}
