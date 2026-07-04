package elasticsearch

import (
	"reflect"
	"testing"

	"log-search/internal/storage"
)

func TestBuildESQueryUsesStructuredFilters(t *testing.T) {
	got := buildESQuery(storage.QueryFilters{
		Env:     "test",
		App:     "api-server",
		Levels:  []string{"warn", "error"},
		Keyword: "timeout",
		TraceID: "tid-0001",
	})

	must := got["bool"].(map[string]interface{})["must"].([]interface{})
	want := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"env.keyword": "test"}},
		map[string]interface{}{"term": map[string]interface{}{"app.keyword": "api-server"}},
		map[string]interface{}{"terms": map[string]interface{}{"level.keyword": []string{"warn", "error"}}},
		map[string]interface{}{"match_phrase": map[string]interface{}{"message": "timeout"}},
		map[string]interface{}{"term": map[string]interface{}{"tid.keyword": "tid-0001"}},
	}
	if !reflect.DeepEqual(must, want) {
		t.Fatalf("must = %#v, want %#v", must, want)
	}
}

func TestBuildESQueryIgnoresInvalidLevels(t *testing.T) {
	got := buildESQuery(storage.QueryFilters{
		Env:    "test",
		Levels: []string{"bad", "INFO", "error"},
	})

	must := got["bool"].(map[string]interface{})["must"].([]interface{})
	want := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"env.keyword": "test"}},
		map[string]interface{}{"terms": map[string]interface{}{"level.keyword": []string{"info", "error"}}},
	}
	if !reflect.DeepEqual(must, want) {
		t.Fatalf("must = %#v, want %#v", must, want)
	}
}

func TestBuildESQueryKeywordAnd(t *testing.T) {
	got := buildESQuery(storage.QueryFilters{
		Env:     "test",
		Keyword: "timeout && retry",
	})

	must := got["bool"].(map[string]interface{})["must"].([]interface{})
	want := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"env.keyword": "test"}},
		map[string]interface{}{"match_phrase": map[string]interface{}{"message": "timeout"}},
		map[string]interface{}{"match_phrase": map[string]interface{}{"message": "retry"}},
	}
	if !reflect.DeepEqual(must, want) {
		t.Fatalf("must = %#v, want %#v", must, want)
	}
}

func TestBuildESQueryKeywordOr(t *testing.T) {
	got := buildESQuery(storage.QueryFilters{
		Env:     "test",
		Keyword: "timeout || retry",
	})

	must := got["bool"].(map[string]interface{})["must"].([]interface{})
	want := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"env.keyword": "test"}},
		map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []interface{}{
					map[string]interface{}{"match_phrase": map[string]interface{}{"message": "timeout"}},
					map[string]interface{}{"match_phrase": map[string]interface{}{"message": "retry"}},
				},
				"minimum_should_match": 1,
			},
		},
	}
	if !reflect.DeepEqual(must, want) {
		t.Fatalf("must = %#v, want %#v", must, want)
	}
}
