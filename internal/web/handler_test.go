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
	labels      []string
	labelValues []string
	result      *loki.QueryResult
	err         error
	lastStart   time.Time
	lastEnd     time.Time
	lastLimit   int
	lastDir     loki.Direction
}

func (m *mockLoki) Labels(ctx context.Context, start, end time.Time) ([]string, error) {
	return m.labels, m.err
}
func (m *mockLoki) LabelValues(ctx context.Context, name string, start, end time.Time) ([]string, error) {
	return m.labelValues, m.err
}
func (m *mockLoki) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction loki.Direction) (*loki.QueryResult, error) {
	m.lastStart = start
	m.lastEnd = end
	m.lastLimit = limit
	m.lastDir = direction
	return m.result, m.err
}

func setupRouter(m loki.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	New(m, 1000).Register(r)
	return r
}

func TestQueryRange_OK_FillsLevel(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{
		Lines: []loki.Line{{Raw: "level=error boom"}},
		Total: 1,
	}}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=foo&start=1&end=99999&limit=10&direction=FORWARD", nil)
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
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999&limit=10&direction=BACKWARD&cursor=5000000000", nil)
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

func TestQueryRange_CursorForward_AdjustsStart(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{}}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999&direction=FORWARD&cursor=5000000000", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999&cursor=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_MissingQuery(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?start=1&end=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BadLimit(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999&limit=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BadRange(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=999&end=999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_LokiError_502(t *testing.T) {
	m := &mockLoki{err: context.DeadlineExceeded}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
}

func TestLabels_OK(t *testing.T) {
	m := &mockLoki{labels: []string{"app", "env"}}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/labels?start=1&end=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		Data []string `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Data) != 2 {
		t.Errorf("data = %v", got.Data)
	}
}
