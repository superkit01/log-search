package elasticsearch

import (
	"context"
	"log-search/internal/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIncompleteSearchIsNotAnEmptySuccess(t *testing.T) {
	for _, body := range []string{`{"timed_out":true,"hits":{"hits":[]}}`, `{"_shards":{"failed":1},"hits":{"hits":[]}}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		client := NewHTTPClient(s.URL, "logs-*", time.Second)
		_, err := client.QueryRange(context.Background(), storage.QueryFilters{Env: "test"}, time.Unix(1, 0), time.Unix(2, 0), 10, storage.Backward)
		s.Close()
		if err == nil || !strings.Contains(err.Error(), "incomplete search") {
			t.Fatalf("%v", err)
		}
	}
}
