// Package assets 用 go:embed 把 web/ 下的模板与静态资源编译进二进制,
// 实现"单文件部署":运行时不再依赖磁盘上的 web/ 目录。
//
// 本包位于仓库根的 web/ 目录(import path "log-search/web"),包名为 assets,
// 避免与 internal/web 的 package web 在同一文件内 import 时撞名。
package assets

import "embed"

// FS 嵌入 web/templates 与 web/static(路径相对本文件所在 web/ 目录)。
// 模板经 internal/web.MountAssets 以 "templates" 子树解析(模板名 index.html);
// 静态资源以 "static" 子树挂到 /static。
//
//go:embed templates static
var FS embed.FS
