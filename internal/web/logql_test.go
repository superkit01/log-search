package web

import "testing"

func TestBuildLogQL(t *testing.T) {
	tests := []struct {
		name string
		req  queryRangeRequest
		want string
	}{
		{
			name: "env app levels keyword trace",
			req: queryRangeRequest{
				Env:     "prod",
				App:     "api",
				Levels:  []string{"info", "warn", "error"},
				Keyword: "timeout",
				TraceID: "t1",
			},
			want: `{env="prod", app="api", level=~"(?i)info|warn|error"} |= "timeout" | tid="t1"`,
		},
		{
			name: "all levels omitted",
			req: queryRangeRequest{
				Env:    "prod",
				Levels: nil,
			},
			want: `{env="prod"}`,
		},
		{
			name: "level only",
			req: queryRangeRequest{
				Levels: []string{"warn"},
			},
			want: `{level=~"(?i)warn"}`,
		},
		{
			name: "escapes values",
			req: queryRangeRequest{
				Env:     `pro"d`,
				App:     `a\b`,
				Keyword: `c"d`,
				TraceID: `e\f`,
			},
			want: `{env="pro\"d", app="a\\b"} |= "c\"d" | tid="e\\f"`,
		},
		{
			name: "invalid level ignored",
			req: queryRangeRequest{
				Env:    "prod",
				Levels: []string{"bad", "error"},
			},
			want: `{env="prod", level=~"(?i)error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildLogQL(tt.req)
			if err != nil {
				t.Fatalf("buildLogQL error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("query = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildLogQL_RequiresSelector(t *testing.T) {
	_, err := buildLogQL(queryRangeRequest{Keyword: "timeout", TraceID: "t1"})
	if err == nil {
		t.Fatal("expected error for missing selector filters")
	}
}
