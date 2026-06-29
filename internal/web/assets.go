package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

// MountAssets 把嵌入的模板与静态资源挂到 gin engine 上,替代磁盘版 LoadHTMLGlob/Static。
// fsys 为 assets.FS(go:embed 产物)。模板从 "templates" 子树解析,模板名为 index.html
// (与 index handler 的 c.HTML(..., "index.html", ...) 对齐);静态资源从 "static" 子树挂到 /static。
func MountAssets(r *gin.Engine, fsys fs.FS) error {
	tmplSub, err := fs.Sub(fsys, "templates")
	if err != nil {
		return fmt.Errorf("sub templates: %w", err)
	}
	tmpl, err := template.ParseFS(tmplSub, "*.html")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	r.SetHTMLTemplate(tmpl)

	staticSub, err := fs.Sub(fsys, "static")
	if err != nil {
		return fmt.Errorf("sub static: %w", err)
	}
	r.StaticFS("/static", http.FS(staticSub))
	return nil
}
