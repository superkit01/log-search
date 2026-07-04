package loki

import (
	"testing"

	"log-search/internal/storage"
)

func TestBuildLogQL(t *testing.T) {
	tests := []struct {
		name    string
		filters storage.QueryFilters
		want    string
	}{
		{
			name: "env app levels keyword trace",
			filters: storage.QueryFilters{
				Env:     "prod",
				App:     "api",
				Levels:  []string{"info", "warn", "error"},
				Keyword: "timeout",
				TraceID: "t1",
			},
			want: `{env="prod", app="api", level=~"(?i)info|warn|error"} |= "timeout" | tid="t1"`,
		},
		{
			name:    "keyword and",
			filters: storage.QueryFilters{Env: "prod", Keyword: "timeout && retry"},
			want:    `{env="prod"} |= "timeout" |= "retry"`,
		},
		{
			name:    "keyword or",
			filters: storage.QueryFilters{Env: "prod", Keyword: `timeout || retry?`},
			want:    `{env="prod"} |~ "timeout|retry\\?"`,
		},
		{
			name:    "all levels omitted",
			filters: storage.QueryFilters{Env: "prod"},
			want:    `{env="prod"}`,
		},
		{
			name:    "level only",
			filters: storage.QueryFilters{Levels: []string{"warn"}},
			want:    `{level=~"(?i)warn"}`,
		},
		{
			name: "escapes values",
			filters: storage.QueryFilters{
				Env:     `pro"d`,
				App:     `a\b`,
				Keyword: `c"d`,
				TraceID: `e\f`,
			},
			want: `{env="pro\"d", app="a\\b"} |= "c\"d" | tid="e\\f"`,
		},
		{
			name: "invalid level ignored",
			filters: storage.QueryFilters{
				Env:    "prod",
				Levels: []string{"bad", "error"},
			},
			want: `{env="prod", level=~"(?i)error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildLogQL(tt.filters)
			if err != nil {
				t.Fatalf("buildLogQL error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("query = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildLogQLRequiresSelector(t *testing.T) {
	_, err := buildLogQL(storage.QueryFilters{Keyword: "timeout", TraceID: "t1"})
	if err == nil {
		t.Fatal("expected error for missing selector filters")
	}
}
