package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log-search/internal/config"
	"log-search/internal/logparse"
	"log-search/internal/loki"

	"github.com/gin-gonic/gin"
)

// Handler 持有 Loki 客户端与默认 limit。
type Handler struct {
	Loki  loki.Client
	Limit int
	UI    UIOptions
}

type UIOptions = config.UICfg

type Choice = config.UIOption

// New 创建 Handler。
func New(lc loki.Client, defaultLimit int, ui ...UIOptions) *Handler {
	opts := UIOptions(config.DefaultUICfg())
	if len(ui) > 0 {
		opts = ui[0]
		if len(opts.Envs) == 0 {
			opts.Envs = config.DefaultUICfg().Envs
		}
	}
	return &Handler{Loki: lc, Limit: defaultLimit, UI: opts}
}

// Register 在 gin engine 上注册路由。
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/", h.index)
	api := r.Group("/api")
	api.POST("/query_range", h.queryRange)
}

func (h *Handler) index(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"envs": h.UI.Envs,
		"apps": h.UI.Apps,
	})
}

func (h *Handler) queryRange(c *gin.Context) {
	var req queryRangeRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxQueryRangeBodyBytes)
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "http: request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must be valid JSON"})
		return
	}
	query, err := buildLogQL(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	start, end, ok := parseRange(c, req.Start, req.End)
	if !ok {
		return
	}
	limit := h.Limit
	direction := loki.Backward
	if req.Direction == string(loki.Forward) {
		direction = loki.Forward
	}
	if req.Cursor != "" {
		ns, err := strconv.ParseInt(req.Cursor, 10, 64)
		if err != nil || ns <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cursor must be a positive unix nanosecond timestamp"})
			return
		}
		if direction == loki.Forward {
			start = time.Unix(0, ns+1)
		} else {
			end = time.Unix(0, ns-1)
		}
		if !end.After(start) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cursor leaves no remaining time range"})
			return
		}
	}
	log.Printf("[Loki Query] query=%q start=%s end=%s limit=%d direction=%s cursor=%s",
		query, start.Format(time.RFC3339), end.Format(time.RFC3339), limit, direction, req.Cursor)
	res, err := h.Loki.QueryRange(c.Request.Context(), query, start, end, limit, direction)
	if err != nil {
		status, body := lokiStatusAndBody(err)
		log.Printf("[Loki Error] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(status, gin.H{"error": body})
		return
	}
	for i := range res.Lines {
		res.Lines[i].Level = string(logparse.DetectLevel(res.Lines[i].Raw))
		res.Lines[i].Labels = normalizeLineTags(res.Lines[i].Labels, res.Lines[i].Level)
	}
	if len(res.Lines) >= limit {
		res.NextCursor = strconv.FormatInt(res.Lines[len(res.Lines)-1].Time.UnixNano(), 10)
	}
	c.JSON(http.StatusOK, res)
}

const maxQueryRangeBodyBytes int64 = 16 * 1024

// logTagKeys is the single allowlist for tags returned to the browser. Add new
// keys here when the UI should display another stable Loki label.
var logTagKeys = []string{"env", "app", "level", "tid"}

func normalizeLineTags(labels map[string]string, level string) map[string]string {
	out := make(map[string]string, len(logTagKeys))
	for _, key := range logTagKeys {
		if key == "level" {
			if level != "" {
				out[key] = level
			}
			continue
		}
		if value := labels[key]; value != "" {
			out[key] = value
		}
	}
	return out
}

// parseRange 解析 start/end(unix 秒),返回 false 表示已写入错误响应。
func parseRange(c *gin.Context, start, end int64) (time.Time, time.Time, bool) {
	if start <= 0 || end <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start and end (unix seconds) are required"})
		return time.Time{}, time.Time{}, false
	}
	st := time.Unix(start, 0)
	en := time.Unix(end, 0)
	if !en.After(st) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end must be after start"})
		return time.Time{}, time.Time{}, false
	}
	return st, en, true
}

// lokiStatusAndBody maps Loki errors to appropriate HTTP status codes and response bodies.
func lokiStatusAndBody(err error) (int, string) {
	var httpErr *loki.HTTPError
	if errors.As(err, &httpErr) {
		// 4xx from Loki → same 4xx to client; 5xx → 502
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			return httpErr.StatusCode, "Loki: " + httpErr.Message
		}
	}
	// Network/timeout or other errors → 502
	return http.StatusBadGateway, "Loki: " + err.Error()
}
