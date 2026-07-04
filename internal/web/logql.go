package web

import (
	"fmt"
	"strings"
)

var levelOrder = []string{"debug", "info", "warn", "error"}

type queryRangeRequest struct {
	Env       string   `json:"env"`
	App       string   `json:"app"`
	Levels    []string `json:"levels"`
	Keyword   string   `json:"keyword"`
	TraceID   string   `json:"traceId"`
	Start     int64    `json:"start"`
	End       int64    `json:"end"`
	Direction string   `json:"direction"`
	Cursor    string   `json:"cursor"`
}

func buildLogQL(req queryRangeRequest) (string, error) {
	matchers := make([]string, 0, 3)
	if env := strings.TrimSpace(req.Env); env != "" {
		matchers = append(matchers, `env="`+escapeLogQLValue(env)+`"`)
	}
	if app := strings.TrimSpace(req.App); app != "" {
		matchers = append(matchers, `app="`+escapeLogQLValue(app)+`"`)
	}
	if lm := levelMatcher(req.Levels); lm != "" {
		matchers = append(matchers, lm)
	}
	if len(matchers) == 0 {
		return "", fmt.Errorf("at least one of env, app, or levels is required")
	}

	parts := []string{"{" + strings.Join(matchers, ", ") + "}"}
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		parts = append(parts, `|= "`+escapeLogQLValue(keyword)+`"`)
	}
	if traceID := strings.TrimSpace(req.TraceID); traceID != "" {
		parts = append(parts, `| tid="`+escapeLogQLValue(traceID)+`"`)
	}
	return strings.Join(parts, " "), nil
}

func levelMatcher(levels []string) string {
	valid := make([]string, 0, len(levels))
	for _, level := range levels {
		level = strings.TrimSpace(strings.ToLower(level))
		if isKnownLevel(level) {
			valid = append(valid, level)
		}
	}
	if len(valid) == 0 {
		return ""
	}
	return `level=~"(?i)` + strings.Join(valid, "|") + `"`
}

func isKnownLevel(level string) bool {
	for _, known := range levelOrder {
		if level == known {
			return true
		}
	}
	return false
}

func escapeLogQLValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}
