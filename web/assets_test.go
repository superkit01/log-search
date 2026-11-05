package assets

import "testing"

// TestFS_HasAssets 确认 go:embed 真的把关键资源打进二进制;
// embed 路径写错或文件被移动时,该测试会在编译期/运行期暴露。
func TestFS_HasAssets(t *testing.T) {
	for _, p := range []string{
		"templates/index.html",
		"static/js/app.js",
		"static/css/style.css",
		"static/vendor/chart.umd.min.js",
		"static/js/levels.js",
		"static/js/share.js",
		"static/js/trace.js",
		"static/js/timeRange.js",
		"static/js/pagination.js",
	} {
		if _, err := FS.ReadFile(p); err != nil {
			t.Errorf("embedded asset missing: %s: %v", p, err)
		}
	}
}
