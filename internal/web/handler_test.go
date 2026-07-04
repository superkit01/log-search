package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log-search/internal/loki"

	"github.com/gin-gonic/gin"
)

type mockLoki struct {
	result    *loki.QueryResult
	err       error
	lastQuery string
	lastStart time.Time
	lastEnd   time.Time
	lastLimit int
	lastDir   loki.Direction
}

func (m *mockLoki) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction loki.Direction) (*loki.QueryResult, error) {
	m.lastQuery = query
	m.lastStart = start
	m.lastEnd = end
	m.lastLimit = limit
	m.lastDir = direction
	return m.result, m.err
}

func setupRouter(m loki.Client) *gin.Engine {
	return setupRouterLimit(m, 1000)
}

func setupRouterLimit(m loki.Client, limit int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	New(m, limit).Register(r)
	return r
}

func postQueryRange(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/query_range", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestQueryRange_OK_FillsLevel(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{
		Lines: []loki.Line{{Raw: "level=error boom"}},
		Total: 1,
	}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","app":"api","levels":["warn","error"],"keyword":"timeout","traceId":"abc123","start":1,"end":99999,"direction":"FORWARD"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res loki.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 1 || res.Lines[0].Level != "error" {
		t.Errorf("unexpected result: %+v", res)
	}
	if m.lastQuery != `{env="prod", app="api", level=~"(?i)warn|error"} |= "timeout" | tid="abc123"` {
		t.Fatalf("query = %q", m.lastQuery)
	}
	if m.lastLimit != 1000 {
		t.Fatalf("limit = %d, want 1000", m.lastLimit)
	}
	if m.lastDir != loki.Forward {
		t.Fatalf("direction = %s, want FORWARD", m.lastDir)
	}
}

func TestQueryRange_NormalizesTags(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{
		Lines: []loki.Line{{
			Raw: "level=warn timeout",
			Labels: map[string]string{
				"env":      "test",
				"app":      "api",
				"level":    "info",
				"tid":      "tid-1",
				"pod":      "api-123",
				"trace_id": "legacy",
			},
		}},
		Total: 1,
	}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"test","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res loki.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	got := res.Lines[0].Labels
	want := map[string]string{
		"env":   "test",
		"app":   "api",
		"level": "warn",
		"tid":   "tid-1",
	}
	if len(got) != len(want) {
		t.Fatalf("labels = %#v, want only %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("labels[%q] = %q, want %q; labels=%#v", key, got[key], value, got)
		}
	}
}

func TestQueryRange_CursorBackward_AdjustsEndAndReturnsNextCursor(t *testing.T) {
	last := time.Unix(0, 1234567890).UTC()
	m := &mockLoki{result: &loki.QueryResult{
		Lines: []loki.Line{
			{Time: time.Unix(0, 2234567890).UTC(), Raw: "info newest"},
			{Time: last, Raw: "info oldest"},
		},
		Total: 2,
	}}
	r := setupRouterLimit(m, 2)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999,"direction":"BACKWARD","cursor":"5000000000"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got, want := m.lastEnd.UnixNano(), int64(4999999999); got != want {
		t.Fatalf("end ns = %d, want %d", got, want)
	}
	var res loki.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.NextCursor != "1234567890" {
		t.Fatalf("nextCursor = %q, want 1234567890", res.NextCursor)
	}
}

func TestQueryRange_NoNextCursorWhenPageIsNotFull(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{
		Lines: []loki.Line{{Time: time.Unix(0, 1234567890).UTC(), Raw: "info only"}},
		Total: 1,
	}}
	r := setupRouterLimit(m, 2)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res loki.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.NextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty", res.NextCursor)
	}
}

func TestQueryRange_CursorForward_AdjustsStart(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999,"direction":"FORWARD","cursor":"5000000000"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got, want := m.lastStart.UnixNano(), int64(5000000001); got != want {
		t.Fatalf("start ns = %d, want %d", got, want)
	}
}

func TestQueryRange_BadCursor(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := postQueryRange(`{"env":"prod","start":1,"end":99999,"cursor":"abc"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_MissingFilter(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := postQueryRange(`{"start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BadRange(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := postQueryRange(`{"env":"prod","start":999,"end":999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BodyTooLarge(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := postQueryRange(`{"env":"prod","start":1,"end":99999,"keyword":"` + strings.Repeat("x", int(maxQueryRangeBodyBytes)) + `"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 body=%s", w.Code, w.Body.String())
	}
}

func TestQueryRange_LokiError_502(t *testing.T) {
	m := &mockLoki{err: context.DeadlineExceeded}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Loki") {
		t.Errorf("body should mention Loki: %s", w.Body.String())
	}
}

func TestQueryRange_LokiHTTPError_400(t *testing.T) {
	m := &mockLoki{err: &loki.HTTPError{StatusCode: http.StatusBadRequest, Message: "parse error"}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Loki") {
		t.Errorf("body should mention Loki: %s", w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] == nil {
		t.Errorf("response should have error field, got %v", resp)
	}
}

func TestQueryRange_LokiHTTPError_502(t *testing.T) {
	// 5xx errors from Loki should still return 502
	m := &mockLoki{err: &loki.HTTPError{StatusCode: http.StatusServiceUnavailable, Message: "Loki overloaded"}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
}
