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

// 以下是 Loki JSON 响应的内部结构(不导出)。

type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	ErrorMsg  string          `json:"error,omitempty"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values []logEntry        `json:"values"` // [unixNanoString, line, optional structured metadata]
}

type logEntry struct {
	Timestamp string
	Line      string
	Metadata  map[string]string
}

func (e *logEntry) UnmarshalJSON(b []byte) error {
	var parts []json.RawMessage
	if err := json.Unmarshal(b, &parts); err != nil {
		return err
	}
	if len(parts) < 2 {
		return nil
	}
	if err := json.Unmarshal(parts[0], &e.Timestamp); err != nil {
		return err
	}
	if err := json.Unmarshal(parts[1], &e.Line); err != nil {
		return err
	}
	if len(parts) > 2 {
		_ = json.Unmarshal(parts[2], &e.Metadata)
	}
	return nil
}

// queryRangeResponse 对应 Loki /query_range 的 data 字段:流数组在 result 里,
// 而不是 data 本身(labels / label values 接口的 data 才是裸数组)。
type queryRangeResponse struct {
	ResultType string   `json:"resultType"`
	Result     []stream `json:"result"`
}
