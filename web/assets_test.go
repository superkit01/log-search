package assets

import "testing"

// TestFS_HasAssets 确认 go:embed 真的把关键资源打进二进制;
// embed 路径写错或文件被移动时,该测试会在编译期/运行期暴露。
func TestFS_HasAssets(t *testing.T) {
	for _, p := range []string{
		"templates/index.html",
		"static/app.js",
		"static/style.css",
		"static/logql.js",
		"static/share.js",
		"static/trace.js",
		"static/timeRange.js",
		"static/pagination.js",
	} {
		if _, err := FS.ReadFile(p); err != nil {
			t.Errorf("embedded asset missing: %s: %v", p, err)
		}
	}
}
