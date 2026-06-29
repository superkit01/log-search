package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	assets "log-search/web"

	"github.com/gin-gonic/gin"
)

// TestMountAssets_ServesIndexAndStatic 验证嵌入资源经 gin 正确提供:
// GET / 渲染 index.html,GET /static/app.js 返回嵌入的 JS。
func TestMountAssets_ServesIndexAndStatic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := MountAssets(r, assets.FS); err != nil {
		t.Fatalf("MountAssets: %v", err)
	}
	New(&mockLoki{}, 1000).Register(r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "日志查询") {
		t.Errorf("GET / body not index.html: %s", w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /static/app.js status = %d, want 200", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "search") {
		t.Errorf("GET /static/app.js body unexpected: %s", w2.Body.String())
	}
}
