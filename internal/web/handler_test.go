package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"log-search/internal/storage"

	"github.com/gin-gonic/gin"
)

type mockStorage struct {
	result      *storage.QueryResult
	err         error
	lastFilters storage.QueryFilters
	lastStart   time.Time
	lastEnd     time.Time
	lastLimit   int
	lastDir     storage.Direction
}

func (m *mockStorage) QueryRange(ctx context.Context, filters storage.QueryFilters, start, end time.Time, limit int, direction storage.Direction) (*storage.QueryResult, error) {
	m.lastFilters = filters
	m.lastStart = start
	m.lastEnd = end
	m.lastLimit = limit
	m.lastDir = direction
	return m.result, m.err
}

func setupRouter(m storage.Client) *gin.Engine {
	return setupRouterLimit(m, 1000)
}

func setupRouterLimit(m storage.Client, limit int) *gin.Engine {
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
	m := &mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{{Raw: "error boom", Labels: map[string]string{"level": "error"}}},
		Total: 1,
	}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","app":"api","levels":["warn","error"],"keyword":"timeout","traceId":"abc123","spanId":" span-1 ","parentSpanId":" parent-1 ","taskId":" task-1 ","messageId":" message-1 ","start":1,"end":99999,"direction":"FORWARD"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res storage.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 1 || res.Lines[0].Level != "error" {
		t.Errorf("unexpected result: %+v", res)
	}
	wantFilters := storage.QueryFilters{
		Env:     "prod",
		App:     "api",
		Levels:  []string{"warn", "error"},
		Keyword: "timeout",
		TraceID: "abc123",
		SpanID:  "span-1", ParentSpanID: "parent-1", TaskID: "task-1", MessageID: "message-1",
	}
	if !reflect.DeepEqual(m.lastFilters, wantFilters) {
		t.Fatalf("filters = %#v, want %#v", m.lastFilters, wantFilters)
	}
	if m.lastLimit != 1000 {
		t.Fatalf("limit = %d, want 1000", m.lastLimit)
	}
	if m.lastDir != storage.Forward {
		t.Fatalf("direction = %s, want FORWARD", m.lastDir)
	}
}

// TestQueryRange_NormalizesUppercaseLevels verifies that uppercase level values
// from Loki/ES are normalized to lowercase for consistent frontend handling.
// This fixes the chart bug where all levels showed as "info" because
// CHART_LEVELS only contains lowercase values.
func TestQueryRange_NormalizesUppercaseLevels(t *testing.T) {
	m := &mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{
			{Raw: "error occurred", Labels: map[string]string{"level": "ERROR"}},
			{Raw: "warning issued", Labels: map[string]string{"level": "WARN"}},
			{Raw: "info message", Labels: map[string]string{"level": "INFO"}},
			{Raw: "debug trace", Labels: map[string]string{"level": "DEBUG"}},
		},
		Total: 4,
	}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"test","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res storage.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	// Verify all uppercase levels are normalized to lowercase
	wantLevels := []string{"error", "warn", "info", "debug"}
	for i, line := range res.Lines {
		if line.Level != wantLevels[i] {
			t.Errorf("line[%d].Level = %q, want %q", i, line.Level, wantLevels[i])
		}
	}
}

func TestQueryRange_NormalizesTags(t *testing.T) {
	m := &mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{{
			Raw: "timeout",
			Labels: map[string]string{
				"env":     "test",
				"app":     "api",
				"level":   "info",
				"tid":     "tid-1",
				"traceId": "tid-1", "spanId": "span-1", "parentSpanId": "parent-1", "taskId": "task-1", "messageId": "message-1",
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
	var res storage.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	got := res.Lines[0].Labels
	want := map[string]string{
		"env":     "test",
		"app":     "api",
		"level":   "info",
		"tid":     "tid-1",
		"traceId": "tid-1", "spanId": "span-1", "parentSpanId": "parent-1", "taskId": "task-1", "messageId": "message-1",
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
	m := &mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{
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
	var res storage.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.NextCursor != "1234567890" {
		t.Fatalf("nextCursor = %q, want 1234567890", res.NextCursor)
	}
}

func TestQueryRange_NoNextCursorWhenPageIsNotFull(t *testing.T) {
	m := &mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{{Time: time.Unix(0, 1234567890).UTC(), Raw: "info only"}},
		Total: 1,
	}}
	r := setupRouterLimit(m, 2)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res storage.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.NextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty", res.NextCursor)
	}
}

func TestQueryRange_CursorForward_AdjustsStart(t *testing.T) {
	m := &mockStorage{result: &storage.QueryResult{}}
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
	r := setupRouter(&mockStorage{})
	req := postQueryRange(`{"env":"prod","start":1,"end":99999,"cursor":"abc"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

type pagedMockStorage struct {
	mockStorage
	lastCursor string
}

func (m *pagedMockStorage) QueryRangePage(ctx context.Context, filters storage.QueryFilters, start, end time.Time, limit int, direction storage.Direction, cursor string) (*storage.QueryResult, error) {
	m.lastCursor = cursor
	return m.QueryRange(ctx, filters, start, end, limit, direction)
}

func TestQueryRange_BackendCursorPreservesOriginalRange(t *testing.T) {
	m := &pagedMockStorage{mockStorage: mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{{Time: time.Unix(2, 0), Labels: map[string]string{"sequence": "9007199254740993"}}},
		Page:  &storage.PageInfo{Continuation: "es1:next"},
	}}}
	r := setupRouterLimit(m, 1)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, postQueryRange(`{"env":"prod","start":1,"end":10,"cursor":"es1:previous"}`))
	if w.Code != 200 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if m.lastCursor != "es1:previous" || m.lastStart != time.Unix(1, 0) || m.lastEnd != time.Unix(10, 0) {
		t.Fatalf("cursor/range changed: %+v", m)
	}
	var result storage.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.NextCursor != "es1:next" || result.Lines[0].Labels["sequence"] != "9007199254740993" {
		t.Fatalf("response=%s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "Continuation") {
		t.Fatal("internal page metadata leaked")
	}
	// A full last page with no continuation must not fall back to a time cursor.
	m.result.Page.Continuation = ""
	w = httptest.NewRecorder()
	r.ServeHTTP(w, postQueryRange(`{"env":"prod","start":1,"end":10}`))
	json.Unmarshal(w.Body.Bytes(), &result)
	var last map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &last)
	if _, exists := last["nextCursor"]; exists {
		t.Fatalf("unexpected cursor: %s", w.Body.String())
	}
}

func TestQueryRange_LokiRejectsESCursor(t *testing.T) {
	w := httptest.NewRecorder()
	setupRouter(&mockStorage{}).ServeHTTP(w, postQueryRange(`{"env":"prod","start":1,"end":10,"cursor":"es1:other"}`))
	if w.Code != 400 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestQueryRange_ESLegacyCursorStaysOnLegacyPagination(t *testing.T) {
	m := &pagedMockStorage{mockStorage: mockStorage{result: &storage.QueryResult{
		Lines: []storage.Line{{Time: time.Unix(2, 0)}},
	}}, lastCursor: "not-called"}
	w := httptest.NewRecorder()
	setupRouterLimit(m, 1).ServeHTTP(w, postQueryRange(`{"env":"prod","start":1,"end":10,"cursor":"5000000000"}`))
	if w.Code != 200 || m.lastCursor != "not-called" || m.lastEnd.UnixNano() != 4999999999 {
		t.Fatalf("legacy path: %d %+v", w.Code, m)
	}
	var result storage.QueryResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.NextCursor != "2000000000" {
		t.Fatalf("legacy cursor=%q", result.NextCursor)
	}
}

func TestQueryRange_MissingFilter(t *testing.T) {
	r := setupRouter(&mockStorage{})
	req := postQueryRange(`{"start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BadRange(t *testing.T) {
	r := setupRouter(&mockStorage{})
	req := postQueryRange(`{"env":"prod","start":999,"end":999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BodyTooLarge(t *testing.T) {
	r := setupRouter(&mockStorage{})
	req := postQueryRange(`{"env":"prod","start":1,"end":99999,"keyword":"` + strings.Repeat("x", int(maxQueryRangeBodyBytes)) + `"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 body=%s", w.Code, w.Body.String())
	}
}

func TestQueryRange_LokiError_502(t *testing.T) {
	m := &mockStorage{err: context.DeadlineExceeded}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Storage") {
		t.Errorf("body should mention Storage: %s", w.Body.String())
	}
}

func TestQueryRange_LokiHTTPError_400(t *testing.T) {
	m := &mockStorage{err: &storage.HTTPError{StatusCode: http.StatusBadRequest, Message: "parse error"}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Storage") {
		t.Errorf("body should mention Storage: %s", w.Body.String())
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
	m := &mockStorage{err: &storage.HTTPError{StatusCode: http.StatusServiceUnavailable, Message: "Loki overloaded"}}
	r := setupRouter(m)
	req := postQueryRange(`{"env":"prod","start":1,"end":99999}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
}

func TestQueryRangeNamespaceSelection(t *testing.T) {
	for _, tc := range []struct {
		namespace string
		status    int
		want      string
	}{
		{"", 200, "default"}, {"ems", 200, "ems"}, {"*", 400, ""}, {"other", 400, ""},
	} {
		t.Run(tc.namespace, func(t *testing.T) {
			m := &mockStorage{result: &storage.QueryResult{Lines: []storage.Line{}}}
			r := gin.New()
			New(m, 100, UIOptions{Namespaces: []Choice{{Value: "default"}, {Value: "ems"}}}).Register(r)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, postQueryRange(`{"env":"dev","start":1,"end":99999,"namespace":"`+tc.namespace+`"}`))
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == 200 && m.lastFilters.Namespace != tc.want {
				t.Fatalf("namespace=%q", m.lastFilters.Namespace)
			}
		})
	}
}
