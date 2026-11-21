// Package console 判断进程是否运行于交互式终端前台，
// 用于决定是否向控制台输出启动信息与日志（后台/服务方式运行时静默）。
package console

import (
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Writer 返回控制台输出目标：
// 交互式终端前台运行时为 os.Stdout，否则为 io.Discard（静默运行，日志仍写入日志文件）。
func Writer() io.Writer {
	if Interactive() {
		return os.Stdout
	}
	return io.Discard
}

// Interactive 返回当前进程是否连接交互式终端且位于前台。
//
// 判定顺序：
//  1. Linux：解析 /proc/self/stat，比较进程组与终端前台进程组 —— 可识别
//     `./dcc &`、nohup 等后台运行方式；
//  2. 其他情况（非 Linux 或解析失败）：以 stdout 是否为字符设备兜底判断，
//     覆盖管道、重定向、服务接管等场景。
func Interactive() bool {
	if runtime.GOOS == "linux" {
		if fg, hasTTY, ok := foregroundFromStat(readProcStat()); ok {
			return hasTTY && fg
		}
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func readProcStat() string {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return ""
	}
	return string(data)
}

// foregroundFromStat 解析 /proc/self/stat：
// 返回（自身是否为终端前台进程组、是否连接终端、解析是否成功）。
func foregroundFromStat(s string) (foreground bool, hasTTY bool, ok bool) {
	if s == "" {
		return false, false, false
	}
	// comm 字段（第 2 项）可包含空格与括号，从最后一个 ')' 之后开始解析。
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return false, false, false
	}
	// ')' 之后依次为：state(0) ppid(1) pgrp(2) session(3) tty_nr(4) tpgid(5)
	fields := strings.Fields(s[i+1:])
	if len(fields) < 6 {
		return false, false, false
	}
	ttyNr, err := strconv.Atoi(fields[4])
	if err != nil {
		return false, false, false
	}
	if ttyNr == 0 {
		return false, false, true // 无控制终端（服务 / 完全重定向）
	}
	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return false, true, false
	}
	tpgid, err := strconv.Atoi(fields[5])
	if err != nil {
		return false, true, false
	}
	return pgrp == tpgid && tpgid != 0, true, true
}
