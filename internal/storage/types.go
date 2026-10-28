package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Client 抽象存储查询能力,便于测试注入 mock。
type Client interface {
	QueryRange(ctx context.Context, filters QueryFilters, start, end time.Time, limit int, direction Direction) (*QueryResult, error)
}

// QueryFilters 是页面筛选条件的存储无关表示。具体后端自行把它转换为
// Loki LogQL 或 Elasticsearch DSL。
type QueryFilters struct {
	Namespace    string
	Env          string
	App          string
	Levels       []string
	Keyword      string
	SpanID       string
	ParentSpanID string
	TaskID       string
	TraceID      string
}

type KeywordOperator string

const (
	KeywordSingle KeywordOperator = "single"
	KeywordAnd    KeywordOperator = "and"
	KeywordOr     KeywordOperator = "or"
)

type KeywordExpression struct {
	Operator KeywordOperator
	Terms    []string
}

func ParseKeyword(keyword string) KeywordExpression {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return KeywordExpression{}
	}
	hasAnd := strings.Contains(keyword, "&&")
	hasOr := strings.Contains(keyword, "||")
	if hasAnd && hasOr {
		return KeywordExpression{Operator: KeywordSingle, Terms: []string{keyword}}
	}
	if hasAnd {
		return keywordExpression(KeywordAnd, strings.Split(keyword, "&&"))
	}
	if hasOr {
		return keywordExpression(KeywordOr, strings.Split(keyword, "||"))
	}
	return KeywordExpression{Operator: KeywordSingle, Terms: []string{keyword}}
}

func keywordExpression(op KeywordOperator, parts []string) KeywordExpression {
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			terms = append(terms, part)
		}
	}
	if len(terms) <= 1 {
		if len(terms) == 0 {
			return KeywordExpression{}
		}
		return KeywordExpression{Operator: KeywordSingle, Terms: terms}
	}
	return KeywordExpression{Operator: op, Terms: terms}
}

func (f QueryFilters) KeywordExpression() KeywordExpression {
	return ParseKeyword(f.Keyword)
}

var levelOrder = []string{"debug", "info", "warn", "error"}

// NormalizedLevels returns known levels in input order, lowercased.
func (f QueryFilters) NormalizedLevels() []string {
	levels := make([]string, 0, len(f.Levels))
	for _, level := range f.Levels {
		level = strings.TrimSpace(strings.ToLower(level))
		if isKnownLevel(level) {
			levels = append(levels, level)
		}
	}
	return levels
}

func (f QueryFilters) HasSelector() bool {
	return strings.TrimSpace(f.Env) != "" || strings.TrimSpace(f.App) != "" || len(f.NormalizedLevels()) > 0
}

func isKnownLevel(level string) bool {
	for _, known := range levelOrder {
		if level == known {
			return true
		}
	}
	return false
}

// Line 是拍平后的单条日志。Level 由上层(web)填充。
type Line struct {
	Index  string            `json:"index,omitempty"`
	ID     string            `json:"id,omitempty"`
	Time   time.Time         `json:"ts"`
	Level  string            `json:"level"`
	Raw    string            `json:"line"`
	Labels map[string]string `json:"labels"`
}

// QueryResult 是范围查询的聚合结果。
type QueryResult struct {
	Lines      []Line `json:"lines"`
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Direction 是查询方向。
type Direction string

const (
	Forward  Direction = "FORWARD"
	Backward Direction = "BACKWARD"
)

// HTTPError is returned when storage responds with a non-success HTTP status.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("storage: HTTP %d: %s", e.StatusCode, e.Message)
}
