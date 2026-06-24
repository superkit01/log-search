package logparse

import (
	"regexp"
	"strings"
)

// Level 表示日志级别。值为小写字符串(与前端 CSS class 后缀一致)。
type Level string

const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
	LevelDebug Level = "debug"
)

// CSSClass 返回前端着色用的 class 名。
func (l Level) CSSClass() string {
	switch l {
	case LevelError:
		return "lvl-error"
	case LevelWarn:
		return "lvl-warn"
	case LevelDebug:
		return "lvl-debug"
	default:
		return "lvl-info"
	}
}

var (
	// 结构化: "level":"error" / level=error / lvl=x / severity=x
	structuredRe = regexp.MustCompile(`(?i)(?:level|levelname|lvl|severity)["']?\s*[:=]\s*["']?([a-z]+)`)
	// 文本: [ERROR] / ERROR / WARN / INFO / DEBUG
	textRe = regexp.MustCompile(`(?i)\b(ERROR|ERR|FATAL|PANIC|WARN(?:ING)?|INFO|DEBUG|TRACE)\b`)
)

// DetectLevel 从单行日志文本中解析级别,无匹配返回 LevelInfo。
func DetectLevel(line string) Level {
	if s := structuredRe.FindStringSubmatch(line); s != nil {
		if lv := normalize(s[1]); lv != "" {
			return lv
		}
	}
	if t := textRe.FindStringSubmatch(line); t != nil {
		if lv := normalize(t[1]); lv != "" {
			return lv
		}
	}
	return LevelInfo
}

// normalize 把捕获到的词归一化为 Level;非级别词返回空字符串。
func normalize(s string) Level {
	switch strings.ToLower(s) {
	case "error", "err", "fatal", "panic":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "info", "information":
		return LevelInfo
	case "debug", "trace":
		return LevelDebug
	}
	return ""
}
