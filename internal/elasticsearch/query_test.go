package elasticsearch

import (
	"reflect"
	"strings"
	"testing"

	"log-search/internal/storage"
)

func TestBuildESQueryUsesStructuredFilters(t *testing.T) {
	got := buildESQuery(storage.QueryFilters{
		Env:       "test",
		App:       "api-server",
		Levels:    []string{"warn", "error"},
		Keyword:   "timeout",
		TraceID:   "tid-0001",
		MessageID: " message-0001 ",
	})

	must := got["bool"].(map[string]interface{})["must"].([]interface{})
	want := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"env": "test"}},
		map[string]interface{}{"term": map[string]interface{}{"app.keyword": "api-server"}},
		map[string]interface{}{"terms": map[string]interface{}{"level": []string{"WARN", "warn", "ERROR", "error"}}},
		map[string]interface{}{"match_phrase": map[string]interface{}{"message": "timeout"}},
		map[string]interface{}{"term": map[string]interface{}{"traceId": "tid-0001"}},
		map[string]interface{}{"term": map[string]interface{}{"messageId": "message-0001"}},
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
		map[string]interface{}{"term": map[string]interface{}{"env": "test"}},
		map[string]interface{}{"terms": map[string]interface{}{"level": []string{"INFO", "info", "ERROR", "error"}}},
	}
	if !reflect.DeepEqual(must, want) {
		t.Fatalf("must = %#v, want %#v", must, want)
	}
}

func TestBuildESQueryLevelCaseCompatibility(t *testing.T) {
	for _, field := range []string{"level", "level.keyword"} {
		for _, uppercase := range []bool{false, true} {
			for _, selected := range [][]string{{"INFO"}, {"warn", "ERROR"}, nil} {
				got := buildESQueryWithFields(storage.QueryFilters{Levels: selected}, FieldMapping{Level: field, UppercaseLevels: uppercase})
				must := got["bool"].(map[string]interface{})["must"].([]interface{})
				if len(selected) == 0 {
					if len(must) != 0 {
						t.Fatalf("all levels added a filter: %#v", must)
					}
					continue
				}
				values := must[0].(map[string]interface{})["terms"].(map[string]interface{})[field].([]string)
				for _, stored := range []string{"info", "INFO", "warn", "WARN", "error", "ERROR", "debug", "DEBUG"} {
					matched := false
					for _, value := range values {
						matched = matched || value == stored
					}
					want := false
					for _, level := range selected {
						want = want || strings.EqualFold(level, stored)
					}
					if matched != want {
						t.Fatalf("field=%s uppercase=%v selected=%v stored=%s: matched=%v", field, uppercase, selected, stored, matched)
					}
				}
			}
		}
	}
}

func TestBuildESQueryKeywordAnd(t *testing.T) {
	got := buildESQuery(storage.QueryFilters{
		Env:     "test",
		Keyword: "timeout && retry",
	})

	must := got["bool"].(map[string]interface{})["must"].([]interface{})
	want := []interface{}{
		map[string]interface{}{"term": map[string]interface{}{"env": "test"}},
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
		map[string]interface{}{"term": map[string]interface{}{"env": "test"}},
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
