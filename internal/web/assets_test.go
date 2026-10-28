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
	New(&mockStorage{}, 1000, UIOptions{
		Envs: []Choice{{Label: "预发", Value: "staging"}},
		Apps: []string{"alert-service"},
	}).Register(r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "日志查询") {
		t.Errorf("GET / body not index.html: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `data-value="staging"`) {
		t.Errorf("GET / body missing configured env: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `value="alert-service"`) {
		t.Errorf("GET / body missing configured app: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `LOG_SEARCH_CONFIG`) || strings.Contains(w.Body.String(), `defaultLimit`) {
		t.Errorf("GET / body should not expose default limit: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "https://cdn.jsdelivr.net") {
		t.Errorf("GET / body should use embedded Chart.js, got CDN reference: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `/static/chart.umd.min.js`) {
		t.Errorf("GET / body missing embedded Chart.js reference: %s", w.Body.String())
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

func TestIndex_EmptyUIOptionsFallsBackToDefaultEnv(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := MountAssets(r, assets.FS); err != nil {
		t.Fatalf("MountAssets: %v", err)
	}
	New(&mockStorage{}, 1000, UIOptions{}).Register(r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `data-value="demo"`) {
		t.Errorf("GET / body missing fallback env: %s", w.Body.String())
	}
}

func TestIndexNamespaceApps(t *testing.T) {
	r := gin.New()
	if err := MountAssets(r, assets.FS); err != nil {
		t.Fatal(err)
	}
	New(&mockStorage{}, 100, UIOptions{Namespaces: []Choice{
		{Value: "default", Label: "默认", Apps: []string{"gateway"}},
		{Value: "pts", Label: "试验", Apps: []string{"worker"}},
	}, Apps: []string{"legacy"}}).Register(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	for _, option := range []string{`data-namespace="default" value="gateway"`, `data-namespace="pts" value="worker"`} {
		if !strings.Contains(w.Body.String(), option) {
			t.Errorf("missing %s", option)
		}
	}
	if strings.Contains(w.Body.String(), `value="legacy"`) {
		t.Error("global apps should not appear in scoped list")
	}
}
