package storage

import (
	"reflect"
	"testing"
)

func TestParseKeyword(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want KeywordExpression
	}{
		{
			name: "single keyword",
			in:   "timeout",
			want: KeywordExpression{Operator: KeywordSingle, Terms: []string{"timeout"}},
		},
		{
			name: "and keyword",
			in:   "timeout && retry",
			want: KeywordExpression{Operator: KeywordAnd, Terms: []string{"timeout", "retry"}},
		},
		{
			name: "or keyword",
			in:   "timeout || retry",
			want: KeywordExpression{Operator: KeywordOr, Terms: []string{"timeout", "retry"}},
		},
		{
			name: "empty parts are ignored",
			in:   "timeout &&  ",
			want: KeywordExpression{Operator: KeywordSingle, Terms: []string{"timeout"}},
		},
		{
			name: "mixed operators stay literal",
			in:   "timeout && retry || failed",
			want: KeywordExpression{Operator: KeywordSingle, Terms: []string{"timeout && retry || failed"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseKeyword(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseKeyword(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
