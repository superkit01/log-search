package loki

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
		  "data":{
		    "resultType":"streams",
		    "result":[{
		      "stream":{"app":"nginx"},
		      "values":[["1609459200000000000","level=info started"],["1609459201000000000","level=error boom"]]
		    }]
		  }
		}`)
	})
	res, err := c.QueryRange(context.Background(), `{app="nginx"}`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d want 2", res.Total)
	}
	// direction=Backward → newest first, so line0 is the later "error" line.
	if res.Lines[0].Raw != "level=error boom" {
		t.Errorf("line0 raw = %q", res.Lines[0].Raw)
	}
	if res.Lines[0].Labels["app"] != "nginx" {
		t.Errorf("labels = %v", res.Lines[0].Labels)
	}
	if res.Lines[0].Level != "" {
		t.Errorf("client must not set level, got %q", res.Lines[0].Level)
	}
}

func TestQueryRange_MergesStructuredMetadataIntoLabels(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
		  "status":"success",
		  "data":{
		    "resultType":"streams",
		    "result":[{
		      "stream":{"env":"test","app":"alarm"},
		      "values":[["1609459200000000000","started",{"trace_id":"tid-1","span_id":"span-1"}]]
		    }]
		  }
		}`)
	})
	res, err := c.QueryRange(context.Background(), `{app="alarm"}`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Lines[0].Labels["trace_id"]; got != "tid-1" {
		t.Fatalf("trace_id label = %q, want tid-1; labels=%v", got, res.Lines[0].Labels)
	}
	if got := res.Lines[0].Labels["app"]; got != "alarm" {
		t.Fatalf("app label = %q, want alarm; labels=%v", got, res.Lines[0].Labels)
	}
}

// TestQueryRange_SortsAcrossStreams verifies the global time merge: Loki returns
// lines grouped per stream, but the client must flatten them into a single
// chronologically-ordered list. FORWARD = oldest first, BACKWARD = newest first.
func TestQueryRange_SortsAcrossStreams(t *testing.T) {
	body := `{
	  "status":"success",
	  "data":{
	    "resultType":"streams",
	    "result":[
	      {"stream":{"app":"a"},"values":[["100","line-100"],["300","line-300"]]},
	      {"stream":{"app":"b"},"values":[["200","line-200"],["400","line-400"]]}
	    ]
	  }
	}`
	for _, tc := range []struct {
		name string
		dir  Direction
		want []string
	}{
		{"forward_ascending", Forward, []string{"line-100", "line-200", "line-300", "line-400"}},
		{"backward_descending", Backward, []string{"line-400", "line-300", "line-200", "line-100"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
			})
			res, err := c.QueryRange(context.Background(), `{app="a"}`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			if res.Total != 4 {
				t.Fatalf("total = %d want 4", res.Total)
			}
			for i, want := range tc.want {
				if res.Lines[i].Raw != want {
					var got []string
					for _, l := range res.Lines {
						got = append(got, l.Raw)
					}
					t.Errorf("dir=%s order = %v, want %v", tc.dir, got, tc.want)
					break
				}
			}
		})
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

// TestQueryRange_PlainTextError reproduces the real-world bug: Loki answers a
// malformed LogQL query with HTTP 400 and a PLAIN-TEXT body
// ("parse error at line 1, col 2: ..."), NOT JSON. The client must surface a
// *HTTPError carrying the status code and body, not a json decode error —
// otherwise the web layer cannot map it to a 400 and falls back to 502.
func TestQueryRange_PlainTextError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "parse error at line 1, col 2: syntax error: unexpected IDENTIFIER")
	})
	_, err := c.QueryRange(context.Background(), `{app="nginx"}`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError so the web layer can map to 4xx, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Message, "parse error at line 1, col 2") {
		t.Errorf("Message = %q, want it to contain Loki's parse error text", httpErr.Message)
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
