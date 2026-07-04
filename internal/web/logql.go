package web

import (
	"strings"

	"log-search/internal/storage"
)

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

func (r queryRangeRequest) filters() storage.QueryFilters {
	return storage.QueryFilters{
		Env:     strings.TrimSpace(r.Env),
		App:     strings.TrimSpace(r.App),
		Levels:  r.Levels,
		Keyword: strings.TrimSpace(r.Keyword),
		TraceID: strings.TrimSpace(r.TraceID),
	}
}
