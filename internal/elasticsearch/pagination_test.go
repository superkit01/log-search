package elasticsearch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"log-search/internal/storage"
)

func TestPITPaginationKeepsSameTimestampHitsAndExactLongValues(t *testing.T) {
	for _, direction := range []storage.Direction{storage.Forward, storage.Backward} {
		t.Run(string(direction), func(t *testing.T) {
			calls, opens, closes := 0, 0, 0
			var firstQuery json.RawMessage
			var previousSort []json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_pit"):
					opens++
					if r.URL.Query().Get("allow_no_indices") != "" {
						t.Error("unsupported ES 7.17 PIT parameter")
					}
					fmt.Fprint(w, `{"id":"pit-0"}`)
				case r.Method == http.MethodDelete && r.URL.Path == "/_pit":
					closes++
					var body map[string]string
					json.NewDecoder(r.Body).Decode(&body)
					if body["id"] != "pit-3" {
						t.Errorf("closed stale PIT: %v", body)
					}
					fmt.Fprint(w, `{"succeeded":true}`)
				case r.URL.Path == "/_search":
					var body struct {
						Query json.RawMessage              `json:"query"`
						Sort  []map[string]json.RawMessage `json:"sort"`
						After []json.RawMessage            `json:"search_after"`
						PIT   struct {
							ID string `json:"id"`
						} `json:"pit"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if calls == 0 {
						firstQuery = body.Query
					} else if string(firstQuery) != string(body.Query) {
						t.Error("query/range changed between pages")
					}
					if !reflect.DeepEqual(body.After, previousSort) {
						t.Errorf("search_after = %s, want %s", body.After, previousSort)
					}
					if body.PIT.ID != fmt.Sprintf("pit-%d", calls) {
						t.Errorf("PIT not refreshed: %s", body.PIT.ID)
					}
					if len(body.Sort) != 3 || string(body.Sort[2]["_shard_doc"]) != `"`+sortOrder(direction)+`"` {
						t.Errorf("sort: %v", body.Sort)
					}
					var sequenceSort map[string]interface{}
					json.Unmarshal(body.Sort[1]["sequence"], &sequenceSort)
					if sequenceSort["order"] != sortOrder(direction) || sequenceSort["unmapped_type"] != "long" || sequenceSort["missing"] != "_last" {
						t.Errorf("sequence sort=%v", sequenceSort)
					}
					calls++
					if calls == 3 {
						fmt.Fprint(w, `{"pit_id":"pit-3","hits":{"hits":[]}}`)
						return
					}
					sequence := "9007199254740993"
					if calls == 2 {
						sequence = "null"
					} // legacy logs without sequence
					tie := calls
					if direction == storage.Backward {
						tie = 3 - calls
					}
					previousSort = []json.RawMessage{json.RawMessage(`1793268000123`), json.RawMessage(sequence), json.RawMessage(fmt.Sprint(tie))}
					fmt.Fprintf(w, `{"pit_id":"pit-%d","hits":{"hits":[{"_index":"logs","_id":"%d","sort":[1793268000123,%s,%d],"_source":{"@timestamp":"2026-10-29T10:00:00.123Z","message":"line","sequence":%s}}]}}`, calls, tie, sequence, tie, sequence)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := NewHTTPClient(server.URL, "logs-*", time.Second).(storage.CursorClient)
			cursor := ""
			for page := 0; page < 3; page++ {
				result, err := client.QueryRangePage(context.Background(), storage.QueryFilters{Env: "dev"}, time.Unix(1, 0), time.Unix(1800000000, 0), 1, direction, cursor)
				if err != nil {
					t.Fatal(err)
				}
				if page == 0 && result.Lines[0].Labels["sequence"] != "9007199254740993" {
					t.Fatalf("sequence lost precision: %v", result.Lines)
				}
				cursor = result.Page.Continuation
				if (page < 2) != (cursor != "") {
					t.Errorf("page %d cursor: %s", page, cursor)
				}
			}
			if opens != 1 || closes != 1 || calls != 3 {
				t.Fatalf("opens=%d closes=%d searches=%d", opens, closes, calls)
			}
		})
	}
}

func TestPITPaginationRejectsMalformedOrChangedQueryCursor(t *testing.T) {
	client := NewHTTPClient("http://unused.invalid", "logs-*", time.Second).(storage.CursorClient)
	changedQuery := "es1:" + base64.RawURLEncoding.EncodeToString([]byte(`{"pit":"other-pit","query":"different-query","sort":[123,4,5]}`))
	for _, cursor := range []string{"es1:bad", "123", changedQuery, "es1:" + strings.Repeat("a", 12001)} {
		_, err := client.QueryRangePage(context.Background(), storage.QueryFilters{}, time.Unix(1, 0), time.Unix(2, 0), 1, storage.Forward, cursor)
		httpErr, ok := err.(*storage.HTTPError)
		if !ok || httpErr.StatusCode != 400 {
			t.Fatalf("cursor %q err=%v", cursor[:min(len(cursor), 20)], err)
		}
	}
}

func TestPITEmptyIndicesAndExpiredSnapshot(t *testing.T) {
	for _, missingIndices := range []bool{true, false} {
		t.Run(fmt.Sprint(missingIndices), func(t *testing.T) {
			closed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodDelete:
					closed = true
					fmt.Fprint(w, `{"succeeded":true}`)
				case strings.HasSuffix(r.URL.Path, "/_pit"):
					if missingIndices {
						w.WriteHeader(404)
						fmt.Fprint(w, `{"error":{"type":"index_not_found_exception"}}`)
					} else {
						fmt.Fprint(w, `{"id":"expired"}`)
					}
				default:
					w.WriteHeader(404)
					fmt.Fprint(w, `{"error":{"type":"search_context_missing_exception"}}`)
				}
			}))
			defer server.Close()
			client := NewHTTPClient(server.URL, "logs-*", time.Second).(storage.CursorClient)
			res, err := client.QueryRangePage(context.Background(), storage.QueryFilters{Env: "dev"}, time.Unix(1, 0), time.Unix(2, 0), 1, storage.Forward, "")
			if missingIndices {
				if err != nil || res.Page == nil || len(res.Lines) != 0 {
					t.Fatalf("empty indices: %v %v", res, err)
				}
			} else {
				httpErr, ok := err.(*storage.HTTPError)
				if !ok || httpErr.StatusCode != 404 || !strings.Contains(httpErr.Message, "重新查询") || !closed {
					t.Fatalf("expired PIT: %v closed=%v", err, closed)
				}
			}
		})
	}
}
