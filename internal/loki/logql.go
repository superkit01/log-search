package loki

import (
	"fmt"
	"regexp"
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
	switch keyword := filters.KeywordExpression(); keyword.Operator {
	case storage.KeywordSingle, storage.KeywordAnd:
		for _, term := range keyword.Terms {
			parts = append(parts, `|= "`+escapeLogQLValue(term)+`"`)
		}
	case storage.KeywordOr:
		parts = append(parts, `|~ "`+escapeLogQLValue(keywordRegex(keyword.Terms))+`"`)
	}
	if traceID := strings.TrimSpace(filters.TraceID); traceID != "" {
		parts = append(parts, `| tid="`+escapeLogQLValue(traceID)+`"`)
	}
	for _, field := range []struct{ name, value string }{
		{"spanId", filters.SpanID}, {"parentSpanId", filters.ParentSpanID}, {"taskId", filters.TaskID},
	} {
		if value := strings.TrimSpace(field.value); value != "" {
			parts = append(parts, `| `+field.name+`="`+escapeLogQLValue(value)+`"`)
		}
	}

	return strings.Join(parts, " "), nil
}

func escapeLogQLValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}

func keywordRegex(terms []string) string {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		quoted = append(quoted, regexp.QuoteMeta(term))
	}
	return strings.Join(quoted, "|")
}
