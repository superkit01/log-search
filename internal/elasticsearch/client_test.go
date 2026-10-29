package elasticsearch

import (
	"context"
	"encoding/json"
	"log-search/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func logstashOptions() Options {
	return Options{Fields: FieldMapping{Env: "env", App: "app.keyword", Level: "level", TraceID: "traceId", UppercaseLevels: true}, IndexTimezone: "Asia/Shanghai"}
}

func TestLogstashQueryAndConversion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app-log-default-dev-20261028/_search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("ignore_unavailable") != "true" {
			t.Error("missing index handling")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		encoded, _ := json.Marshal(body)
		for _, fragment := range []string{`"env":"dev"`, `"app.keyword":"gateway"`, `"level":["INFO"]`, `"traceId":"abc"`, `"spanId":"span"`, `"parentSpanId":"parent"`, `"taskId":"task"`, `"messageId":"message"`} {
			if !strings.Contains(string(encoded), fragment) {
				t.Errorf("missing %s in %s", fragment, encoded)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits":{"hits":[{"_source":{"@timestamp":"2026-10-28T06:46:56.518Z","message":"request","level":"INFO","traceId":"abc","stack_trace":"exception"}},{"_source":{"message":"bad timestamp"}}]}}`))
	}))
	defer server.Close()
	c := NewHTTPClient(server.URL, "app-log-{namespace}-{env}-{yyyyMMdd}", time.Second, logstashOptions())
	result, err := c.QueryRange(context.Background(), storage.QueryFilters{Namespace: "default", Env: "dev", App: "gateway", Levels: []string{"info"}, TraceID: "abc", SpanID: "span", ParentSpanID: "parent", TaskID: "task", MessageID: "message"}, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC), 100, storage.Backward)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0].Labels["tid"] != "abc" || result.Lines[0].Raw != "request\nexception" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDailyIndicesCoverCalendarBoundaries(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start := time.Date(2026, 10, 28, 23, 30, 0, 0, loc)
	for _, tc := range []struct {
		end  time.Time
		want string
	}{
		{start.Add(time.Hour), "logs-dev-20261028,logs-dev-20261029"},
		{start.Add(49 * time.Hour), "logs-dev-20261028,logs-dev-20261029,logs-dev-20261030,logs-dev-20261031"},
	} {
		if got := buildTimeRangeIndex("logs-{env}-{yyyyMMdd}", "dev", start, tc.end); got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}

func TestLiveLogstash(t *testing.T) {
	endpoint := os.Getenv("LOG_SEARCH_TEST_ES_URL")
	if endpoint == "" {
		t.Skip("set LOG_SEARCH_TEST_ES_URL for read-only integration test")
	}
	c := NewHTTPClient(endpoint, "app-log-default-{env}-{yyyyMMdd}", 20*time.Second, logstashOptions())
	start := time.Date(2026, 10, 27, 16, 0, 0, 0, time.UTC)
	for _, filters := range []storage.QueryFilters{
		{Env: "dev", App: os.Getenv("LOG_SEARCH_TEST_ES_APP"), Levels: []string{"info"}},
		{Env: "dev", TraceID: os.Getenv("LOG_SEARCH_TEST_ES_TRACE_ID")},
	} {
		result, err := c.QueryRange(context.Background(), filters, start, start.Add(24*time.Hour), 1000, storage.Backward)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Lines) == 0 {
			t.Fatalf("no matching logs for %+v", filters)
		}
		t.Logf("matched %d logs", len(result.Lines))
	}
}
