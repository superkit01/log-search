package loki

import (
	"encoding/json"
	"time"
)

// Line 是拍平后的单条日志。Level 由上层(web)填充。
type Line struct {
	Time   time.Time         `json:"ts"`
	Level  string            `json:"level"`
	Raw    string            `json:"line"`
	Labels map[string]string `json:"labels"`
}

// QueryResult 是范围查询的聚合结果。
type QueryResult struct {
	Lines []Line `json:"lines"`
	Total int    `json:"total"`
}

// Direction 是查询方向。
type Direction string

const (
	Forward  Direction = "FORWARD"
	Backward Direction = "BACKWARD"
)

// 以下是 Loki JSON 响应的内部结构(不导出)。

type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	ErrorMsg  string          `json:"error,omitempty"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"` // [unixNanoString, line]
}
