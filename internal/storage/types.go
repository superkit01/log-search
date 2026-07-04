package storage

import (
	"context"
	"fmt"
	"time"
)

// Client 抽象存储查询能力,便于测试注入 mock。
type Client interface {
	QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction Direction) (*QueryResult, error)
}

// Line 是拍平后的单条日志。Level 由上层(web)填充。
type Line struct {
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
