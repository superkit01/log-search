package elasticsearch

import (
	"strings"

	"log-search/internal/storage"
)

func buildESQuery(filters storage.QueryFilters) map[string]interface{} {
	return buildESQueryWithFields(filters, FieldMapping{Env: "env", App: "app.keyword", Level: "level", TraceID: "traceId", UppercaseLevels: true})
}

func buildESQueryWithFields(filters storage.QueryFilters, fields FieldMapping) map[string]interface{} {
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []interface{}{},
		},
	}
	mustList := query["bool"].(map[string]interface{})["must"].([]interface{})

	if namespace := strings.TrimSpace(filters.Namespace); namespace != "" {
		mustList = append(mustList, term("namespace", namespace))
	}
	if env := strings.TrimSpace(filters.Env); env != "" {
		mustList = append(mustList, term(fields.Env, env))
	}
	if app := strings.TrimSpace(filters.App); app != "" {
		mustList = append(mustList, term(fields.App, app))
	}
	if levels := filters.NormalizedLevels(); len(levels) > 0 {
		if fields.UppercaseLevels {
			for i := range levels {
				levels[i] = strings.ToUpper(levels[i])
			}
		}
		mustList = append(mustList, map[string]interface{}{
			"terms": map[string]interface{}{
				fields.Level: levels,
			},
		})
	}
	switch keyword := filters.KeywordExpression(); keyword.Operator {
	case storage.KeywordSingle, storage.KeywordAnd:
		for _, term := range keyword.Terms {
			mustList = append(mustList, matchMessage(term))
		}
	case storage.KeywordOr:
		should := make([]interface{}, 0, len(keyword.Terms))
		for _, term := range keyword.Terms {
			should = append(should, matchMessage(term))
		}
		mustList = append(mustList, map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               should,
				"minimum_should_match": 1,
			},
		})
	}
	if traceID := strings.TrimSpace(filters.TraceID); traceID != "" {
		mustList = append(mustList, term(fields.TraceID, traceID))
	}

	for _, field := range []struct{ name, value string }{
		{"spanId", filters.SpanID}, {"parentSpanId", filters.ParentSpanID}, {"taskId", filters.TaskID},
	} {
		if value := strings.TrimSpace(field.value); value != "" {
			mustList = append(mustList, term(field.name, value))
		}
	}
	query["bool"].(map[string]interface{})["must"] = mustList
	return query
}

func matchMessage(value string) map[string]interface{} {
	return map[string]interface{}{
		"match_phrase": map[string]interface{}{
			"message": value,
		},
	}
}

func term(field, value string) map[string]interface{} {
	return map[string]interface{}{
		"term": map[string]interface{}{
			field: value,
		},
	}
}
