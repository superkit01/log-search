package loki

import (
	"fmt"
	"strings"

	"log-search/internal/storage"
)

func buildLogQL(filters storage.QueryFilters) (string, error) {
	matchers := make([]string, 0, 3)
	if env := strings.TrimSpace(filters.Env); env != "" {
		matchers = append(matchers, `env="`+escapeLogQLValue(env)+`"`)
	}
	if app := strings.TrimSpace(filters.App); app != "" {
		matchers = append(matchers, `app="`+escapeLogQLValue(app)+`"`)
	}
	if levels := filters.NormalizedLevels(); len(levels) > 0 {
		matchers = append(matchers, `level=~"(?i)`+strings.Join(levels, "|")+`"`)
	}
	if len(matchers) == 0 {
		return "", fmt.Errorf("at least one of env, app, or levels is required")
	}

	parts := []string{"{" + strings.Join(matchers, ", ") + "}"}
	if keyword := strings.TrimSpace(filters.Keyword); keyword != "" {
		parts = append(parts, `|= "`+escapeLogQLValue(keyword)+`"`)
	}
	if traceID := strings.TrimSpace(filters.TraceID); traceID != "" {
		parts = append(parts, `| tid="`+escapeLogQLValue(traceID)+`"`)
	}
	return strings.Join(parts, " "), nil
}

func escapeLogQLValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}
