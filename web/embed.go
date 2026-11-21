// Package web 内嵌前端静态资源（HTML/CSS/JS），编译进单一可执行文件。
package web

import "embed"

// Static 前端构建产物（web/static 目录）。
//
//go:embed all:static
var Static embed.FS
