package console

import "testing"

func TestForegroundFromStat(t *testing.T) {
	cases := []struct {
		name       string
		stat       string
		foreground bool
		hasTTY     bool
		ok         bool
	}{
		{
			name: "前台交互运行",
			// pgrp=4321 与 tpgid=4321 相同 → 前台
			stat:       "1234 (dcc) S 1 4321 4321 34816 4321 4194304 100 0 0 0 0 0 0 0 20 0 1 0 100 1000 1000 0 0",
			foreground: true, hasTTY: true, ok: true,
		},
		{
			name: "后台运行（& / nohup）",
			// pgrp=4321 与 tpgid=777 不同 → 后台
			stat:       "1234 (dcc) S 1 4321 4321 34816 777 4194304 100 0 0 0 0 0 0 0 20 0 1 0 100 1000 1000 0 0",
			foreground: false, hasTTY: true, ok: true,
		},
		{
			name: "无控制终端",
			// tty_nr=0 → 服务 / 完全重定向
			stat:       "1234 (dcc) S 1 4321 4321 0 -1 4194304 100 0 0 0 0 0 0 0 20 0 1 0 100 1000 1000 0 0",
			foreground: false, hasTTY: false, ok: true,
		},
		{
			name: "comm 含空格与括号",
			stat:       "1234 (my app (x)) S 1 4321 4321 34816 4321 4194304 100 0 0 0 0 0 0 0 20 0 1 0 100 1000 1000 0 0",
			foreground: true, hasTTY: true, ok: true,
		},
		{
			name: "字段不足",
			stat: "1234 (dcc) S 1",
			ok:   false,
		},
		{
			name: "空输入",
			stat: "",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fg, tty, ok := foregroundFromStat(tc.stat)
			if ok != tc.ok {
				t.Fatalf("ok = %v, 期望 %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if fg != tc.foreground || tty != tc.hasTTY {
				t.Fatalf("foreground=%v hasTTY=%v, 期望 %v / %v", fg, tty, tc.foreground, tc.hasTTY)
			}
		})
	}
}
