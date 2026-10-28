package web

import (
	"strings"

	"log-search/internal/storage"
)

type queryRangeRequest struct {
	Namespace    string   `json:"namespace"`
	Env          string   `json:"env"`
	App          string   `json:"app"`
	Levels       []string `json:"levels"`
	Keyword      string   `json:"keyword"`
	SpanID       string   `json:"spanId"`
	ParentSpanID string   `json:"parentSpanId"`
	TaskID       string   `json:"taskId"`
	TraceID      string   `json:"traceId"`
	Start        int64    `json:"start"`
	End          int64    `json:"end"`
	Direction    string   `json:"direction"`
	Cursor       string   `json:"cursor"`
}

func (r queryRangeRequest) filters() storage.QueryFilters {
	return storage.QueryFilters{
		Namespace:    strings.TrimSpace(r.Namespace),
		Env:          strings.TrimSpace(r.Env),
		App:          strings.TrimSpace(r.App),
		Levels:       r.Levels,
		Keyword:      strings.TrimSpace(r.Keyword),
		TraceID:      strings.TrimSpace(r.TraceID),
		SpanID:       strings.TrimSpace(r.SpanID),
		ParentSpanID: strings.TrimSpace(r.ParentSpanID),
		TaskID:       strings.TrimSpace(r.TaskID),
	}
}
