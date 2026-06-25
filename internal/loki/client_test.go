package loki

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL, 5*time.Second)
}

func TestQueryRange_ParsesStreams(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		fmt.Fprint(w, `{
		  "status":"success",
		  "data":[{
		    "stream":{"app":"nginx"},
		    "values":[["1609459200000000000","level=info started"],["1609459201000000000","level=error boom"]]
		  }]
		}`)
	})
	res, err := c.QueryRange(context.Background(), `{app="nginx"}`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d want 2", res.Total)
	}
	if res.Lines[0].Raw != "level=info started" {
		t.Errorf("line0 raw = %q", res.Lines[0].Raw)
	}
	if res.Lines[0].Labels["app"] != "nginx" {
		t.Errorf("labels = %v", res.Lines[0].Labels)
	}
	if res.Lines[0].Level != "" {
		t.Errorf("client must not set level, got %q", res.Lines[0].Level)
	}
}

func TestLabels(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/labels" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"success","data":["app","env"]}`)
	})
	got, err := c.Labels(context.Background(), time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "app" {
		t.Errorf("labels = %v", got)
	}
}

func TestLabelValues(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/app/values" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"success","data":["nginx","api"]}`)
	})
	got, err := c.LabelValues(context.Background(), "app", time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("values = %v", got)
	}
}

func TestQueryRange_LokiError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"parse error at col 1"}`)
	})
	_, err := c.QueryRange(context.Background(), `{`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestQueryRange_ReturnsHTTPError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"parse error at col 1"}`)
	})
	_, err := c.QueryRange(context.Background(), `{`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", httpErr.StatusCode)
	}
	if !errors.Is(err, httpErr) {
		t.Errorf("errors.As should succeed, got %v", err)
	}
}
