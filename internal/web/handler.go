package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"log-search/internal/logparse"
	"log-search/internal/loki"

	"github.com/gin-gonic/gin"
)

const maxLimit = 5000

// Handler 持有 Loki 客户端与默认 limit。
type Handler struct {
	Loki  loki.Client
	Limit int
	UI    UIOptions
}

type UIOptions struct {
	Envs []Choice
	Apps []string
}

type Choice struct {
	Label string
	Value string
}

// New 创建 Handler。
func New(lc loki.Client, defaultLimit int, ui ...UIOptions) *Handler {
	opts := defaultUIOptions()
	if len(ui) > 0 {
		opts = ui[0]
	}
	return &Handler{Loki: lc, Limit: defaultLimit, UI: opts}
}

// Register 在 gin engine 上注册路由。
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/", h.index)
	api := r.Group("/api")
	api.GET("/labels", h.labels)
	api.GET("/label/:name/values", h.labelValues)
	api.GET("/query_range", h.queryRange)
}

func (h *Handler) index(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"defaultLimit": h.Limit,
		"envs":         h.UI.Envs,
		"apps":         h.UI.Apps,
	})
}

func defaultUIOptions() UIOptions {
	return UIOptions{Envs: []Choice{
		{Label: "演示", Value: "demo"},
	}}
}

func (h *Handler) labels(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		return
	}
	labels, err := h.Loki.Labels(c.Request.Context(), start, end)
	respondData(c, labels, err)
}

func (h *Handler) labelValues(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		return
	}
	vals, err := h.Loki.LabelValues(c.Request.Context(), c.Param("name"), start, end)
	respondData(c, vals, err)
}

func (h *Handler) queryRange(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	start, end, ok := parseRange(c)
	if !ok {
		return
	}
	limit := h.Limit
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and " + strconv.Itoa(maxLimit)})
			return
		}
		limit = n
	}
	direction := loki.Backward
	if c.Query("direction") == string(loki.Forward) {
		direction = loki.Forward
	}
	if s := c.Query("cursor"); s != "" {
		ns, err := strconv.ParseInt(s, 10, 64)
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
	res, err := h.Loki.QueryRange(c.Request.Context(), query, start, end, limit, direction)
	if err != nil {
		status, body := lokiStatusAndBody(err)
		log.Printf("[Loki Error] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(status, gin.H{"error": body})
		return
	}
	for i := range res.Lines {
		res.Lines[i].Level = string(logparse.DetectLevel(res.Lines[i].Raw))
	}
	if len(res.Lines) > 0 {
		res.NextCursor = strconv.FormatInt(res.Lines[len(res.Lines)-1].Time.UnixNano(), 10)
	}
	c.JSON(http.StatusOK, res)
}

// parseRange 解析 start/end(unix 秒),返回 false 表示已写入错误响应。
func parseRange(c *gin.Context) (time.Time, time.Time, bool) {
	start, err1 := strconv.ParseInt(c.Query("start"), 10, 64)
	end, err2 := strconv.ParseInt(c.Query("end"), 10, 64)
	if err1 != nil || err2 != nil {
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

// respondData 统一处理 labels/values 的成功/失败响应。
func respondData(c *gin.Context, data interface{}, err error) {
	if err != nil {
		status, body := lokiStatusAndBody(err)
		log.Printf("[Loki Error] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(status, gin.H{"error": body})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
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
