// Package query owns the shared browser and MCP query contract.
package query

import (
	"context"
	"log-search/internal/storage"
	"strconv"
	"strings"
	"time"
)

type Error struct {
	Status  int
	Message string
}

const MaxLimit = 1000

func (e *Error) Error() string { return e.Message }

type Service struct {
	Client     storage.Client
	Limit      int
	Namespaces []string
}

func (s *Service) Execute(ctx context.Context, req Request) (*storage.QueryResult, error) {
	filters := req.Filters()
	if len(s.Namespaces) > 0 {
		if filters.Namespace == "" {
			filters.Namespace = s.Namespaces[0]
		}
		allowed := false
		for _, option := range s.Namespaces {
			if option == filters.Namespace {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, &Error{Status: 400, Message: "namespace is not configured"}
		}
	} else if filters.Namespace != "" {
		return nil, &Error{Status: 400, Message: "namespace is not configured"}
	}

	if !filters.HasSelector() {
		return nil, &Error{Status: 400, Message: "at least one of env, app, or levels is required"}
	}
	if req.Start <= 0 || req.End <= 0 {
		return nil, &Error{400, "start and end (unix seconds) are required"}
	}
	start, end := time.Unix(req.Start, 0), time.Unix(req.End, 0)
	if !end.After(start) {
		return nil, &Error{400, "end must be after start"}
	}
	limit := s.Limit
	if req.Limit != nil {
		if *req.Limit < 1 || *req.Limit > MaxLimit {
			return nil, &Error{Status: 400, Message: "limit must be between 1 and 1000"}
		}
		limit = *req.Limit
	}
	direction := storage.Backward
	if req.Direction == string(storage.Forward) {
		direction = storage.Forward
	}
	if req.Cursor != "" {
		if strings.HasPrefix(req.Cursor, "es1:") {
			if _, ok := s.Client.(storage.CursorClient); !ok {
				return nil, &Error{Status: 400, Message: "cursor is not supported by this storage backend"}
			}
		} else {
			ns, err := strconv.ParseInt(req.Cursor, 10, 64)
			if err != nil || ns <= 0 {
				return nil, &Error{Status: 400, Message: "cursor must be a positive unix nanosecond timestamp"}
			}
			if direction == storage.Forward {
				start = time.Unix(0, ns+1)
			} else {
				end = time.Unix(0, ns-1)
			}
			if !end.After(start) {
				return nil, &Error{Status: 400, Message: "cursor leaves no remaining time range"}
			}
		}
	}
	var res *storage.QueryResult
	var err error
	if paged, ok := s.Client.(storage.CursorClient); ok && (req.Cursor == "" || strings.HasPrefix(req.Cursor, "es1:")) {
		cursor := ""
		if strings.HasPrefix(req.Cursor, "es1:") {
			cursor = req.Cursor
		}
		res, err = paged.QueryRangePage(ctx, filters, start, end, limit, direction, cursor)
	} else {
		res, err = s.Client.QueryRange(ctx, filters, start, end, limit, direction)
	}
	if err != nil {
		return nil, err
	}
	for i := range res.Lines {
		// 从 labels 中提取 level 到 Level 字段（前端使用 l.level 而非 l.labels["level"]）
		// 规范化为小写，以兼容 Loki/ES 返回的大写 level 值（ERROR, WARN, INFO, DEBUG）
		if lvl, ok := res.Lines[i].Labels["level"]; ok {
			res.Lines[i].Level = strings.ToLower(lvl)
			// 同时更新 labels 中的 level 值以保持一致性
			res.Lines[i].Labels["level"] = res.Lines[i].Level
		}
		res.Lines[i].Labels = normalizeLineTags(res.Lines[i].Labels)
	}
	if res.Page != nil {
		res.NextCursor = res.Page.Continuation
	} else if len(res.Lines) >= limit {
		res.NextCursor = strconv.FormatInt(res.Lines[len(res.Lines)-1].Time.UnixNano(), 10)
	}
	return res, nil
}

// logTagKeys is the single allowlist for tags returned to the browser. Add new
// keys here when the UI should display another stable Loki label.
var logTagKeys = []string{"env", "app", "level", "tid", "traceId", "namespace", "spanId", "parentSpanId", "taskId", "messageId", "sequence", "logger_name", "thread_name", "host"}

func normalizeLineTags(labels map[string]string) map[string]string {
	out := make(map[string]string, len(logTagKeys))
	for _, key := range logTagKeys {
		if value := labels[key]; value != "" {
			out[key] = value
		}
	}
	return out
}
