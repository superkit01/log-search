package agenttools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"log-search/internal/query"
)

func TestQueryPreservesCursorFiltersAndBuildsLink(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/query_range" || r.Method != "POST" {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
		}
		var q query.Request
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.Limit == nil || *q.Limit != 50 {
			t.Error("lost limit")
		}
		if q.TraceID != "trace & 中文" || q.TaskID != "task1" || q.MessageID != "mq1" || q.Namespace != "pts" || q.Env != "test" || q.Start != 100 || q.End != 200 {
			t.Errorf("lost filters: %#v", q)
		}
		if calls == 1 && q.Cursor != "es1:opaque" {
			t.Errorf("lost cursor: %#v", q)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lines":[],"total":0,"nextCursor":"es1:opaque"}`))
	}))
	defer upstream.Close()
	a, _ := New(upstream.URL, time.Second)
	limit := 50
	q := query.Request{Limit: &limit, Namespace: "pts", Env: "test", Start: 100, End: 200, TraceID: "trace & 中文", TaskID: "task1", MessageID: "mq1"}
	for i := 0; i < 2; i++ {
		args, _ := json.Marshal(q)
		value, err := a.Call(context.Background(), "query_logs", args)
		if err != nil {
			t.Fatal(err)
		}
		result := value.(map[string]any)
		if result["hasMore"] != true {
			t.Fatal("missing pagination indication")
		}
		u, _ := url.Parse(result["queryUrl"].(string))
		if u.Query().Get("trace_id") != q.TraceID || u.Query().Get("cursor") != "" || u.Query().Get("limit") != "50" || u.Query().Get("message_id") != "mq1" {
			t.Fatal("invalid query link", u)
		}
		if result["request"].(query.Request).Cursor != q.Cursor {
			t.Fatal("request card differs from actual query")
		}
		q.Cursor = "es1:opaque"
	}
}

func TestValidationErrorsAndOptions(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api/query_options" {
			_, _ = w.Write([]byte(`{"Namespaces":[{"Value":"pts","Envs":[{"Value":"test"}]}]}`))
			return
		}
		w.WriteHeader(410)
		_, _ = w.Write([]byte("private secret upstream body"))
	}))
	defer upstream.Close()
	a, _ := New(upstream.URL, time.Second)
	for _, args := range []string{`{"env":"test","start":1,"end":2,"limit":0}`, `{"env":"test","start":1,"end":2,"limit":1001}`, `{"env":"test","start":1,"end":2,"url":"http://other"}`, `{"env":"test","start":2,"end":1}`, `{"env":"test","start":1,"end":2,"levels":["invalid"]}`} {
		if _, err := a.Call(context.Background(), "query_logs", json.RawMessage(args)); err == nil {
			t.Fatal("accepted invalid args")
		}
	}
	if calls != 0 {
		t.Fatal("invalid args reached network")
	}
	if _, err := a.Call(context.Background(), "get_query_options", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	_, err := a.Call(context.Background(), "query_logs", json.RawMessage(`{"namespace":"pts","env":"test","start":1,"end":2}`))
	if err == nil || !strings.Contains(err.Error(), "410") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
	for _, base := range []string{"file:///etc/passwd", "http://user:password@host", "http://host/?token=secret"} {
		if _, err := New(base, time.Second); err == nil {
			t.Fatal("invalid base URL")
		}
	}
}

func TestCancellation(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(cancelled) }))
	defer server.Close()
	a, _ := New(server.URL, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := a.Call(ctx, "get_query_options", json.RawMessage(`{}`)); done <- err }()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancellation")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("HTTP query was not cancelled")
	}
}
