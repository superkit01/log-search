package elasticsearch

import (
	"strings"

	"log-search/internal/storage"
)

func buildESQuery(filters storage.QueryFilters) map[string]interface{} {
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []interface{}{},
		},
	}
	mustList := query["bool"].(map[string]interface{})["must"].([]interface{})

	if env := strings.TrimSpace(filters.Env); env != "" {
		mustList = append(mustList, term("env.keyword", env))
	}
	if app := strings.TrimSpace(filters.App); app != "" {
		mustList = append(mustList, term("app.keyword", app))
	}
	if levels := filters.NormalizedLevels(); len(levels) > 0 {
		mustList = append(mustList, map[string]interface{}{
			"terms": map[string]interface{}{
				"level.keyword": levels,
			},
		})
	}
	if keyword := strings.TrimSpace(filters.Keyword); keyword != "" {
		mustList = append(mustList, map[string]interface{}{
			"match_phrase": map[string]interface{}{
				"message": keyword,
			},
		})
	}
	if traceID := strings.TrimSpace(filters.TraceID); traceID != "" {
		mustList = append(mustList, term("tid.keyword", traceID))
	}

	query["bool"].(map[string]interface{})["must"] = mustList
	return query
}

func term(field, value string) map[string]interface{} {
	return map[string]interface{}{
		"term": map[string]interface{}{
			field: value,
		},
	}
}
