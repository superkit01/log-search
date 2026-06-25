package logparse

import "testing"

func TestDetectLevel(t *testing.T) {
	cases := []struct {
		in   string
		want Level
	}{
		{`{"level":"error","msg":"x"}`, LevelError},
		{`{"levelname":"WARN","msg":"x"}`, LevelWarn},
		{`{"level":"info"}`, LevelInfo},
		{`{"level":"debug"}`, LevelDebug},
		{`level=error msg=x`, LevelError},
		{`lvl=warn msg=x`, LevelWarn},
		{`severity=info x`, LevelInfo},
		{`[ERROR] connection refused`, LevelError},
		{`2026-06-24 INFO started`, LevelInfo},
		{`FATAL panic`, LevelError},
		{`something went wrong here`, LevelInfo}, // 无匹配 → 默认 INFO
		{`{"msg":"ok","level":"trace"}`, LevelDebug},
	}
	for _, c := range cases {
		got := DetectLevel(c.in)
		if got != c.want {
			t.Errorf("DetectLevel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCSSClass(t *testing.T) {
	cases := map[Level]string{
		LevelError: "lvl-error",
		LevelWarn:  "lvl-warn",
		LevelInfo:  "lvl-info",
		LevelDebug: "lvl-debug",
	}
	for lv, want := range cases {
		if got := lv.CSSClass(); got != want {
			t.Errorf("%v CSSClass = %q, want %q", lv, got, want)
		}
	}
}
