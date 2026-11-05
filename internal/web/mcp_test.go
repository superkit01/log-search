package web

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"log-search/internal/mcp"
	"log-search/internal/storage"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMCPUsesSharedQueryAndHidesConfig(t *testing.T) {
	mock := &mockStorage{result: &storage.QueryResult{Lines: []storage.Line{{Time: time.Unix(150, 0), Raw: "failure", Labels: map[string]string{"level": "ERROR", "private": "hidden"}}}}}
	h := New(mock, 1000, UIOptions{Namespaces: []Choice{{Value: "pts", Envs: []Choice{{Value: "test"}}}}})
	r := gin.New()
	h.Register(r)
	h.RegisterMCP(r, mcp.Config{Enabled: true, Credentials: []mcp.Credential{{Name: "default", Key: "private-mcp-key"}}, ExternalURL: "http://localhost:8080"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"query_logs","arguments":{"namespace":"pts","env":"test","start":100,"end":200,"traceId":"trace1"}}}`))
	req.Header.Set("Authorization", "Bearer private-mcp-key")
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"level":"error"`) || strings.Contains(w.Body.String(), "hidden") {
		t.Fatal(w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["result"].(map[string]any)["isError"] != false {
		t.Fatal(body)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/query_options", nil))
	if strings.Contains(w.Body.String(), "private-mcp-key") {
		t.Fatal("key leaked in options")
	}
	disabled := gin.New()
	h.RegisterMCP(disabled, mcp.Config{})
	w = httptest.NewRecorder()
	disabled.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", nil))
	if w.Code != 404 {
		t.Fatal("disabled route exposed")
	}
}
