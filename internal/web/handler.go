package web

import (
	"errors"
	"net/http"
	"strings"

	"log-search/internal/config"
	"log-search/internal/query"
	"log-search/internal/storage"

	"github.com/gin-gonic/gin"
)

// Handler 持有存储客户端与默认 limit。
type Handler struct {
	Client storage.Client
	Limit  int
	UI     UIOptions
}

type UIOptions = config.UICfg

type Choice = config.UIOption

// New 创建 Handler。
func New(client storage.Client, defaultLimit int, ui ...UIOptions) *Handler {
	opts := UIOptions(config.DefaultUICfg())
	if len(ui) > 0 {
		opts = ui[0]
		if len(opts.Envs) == 0 {
			opts.Envs = config.DefaultUICfg().Envs
		}
	}
	for i := range opts.Namespaces {
		if len(opts.Namespaces[i].Envs) == 0 {
			opts.Namespaces[i].Envs = opts.Envs
		}
	}
	return &Handler{Client: client, Limit: defaultLimit, UI: opts}
}

// Register 在 gin engine 上注册路由。
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/", h.index)
	api := r.Group("/api")
	api.POST("/query_range", h.queryRange)
	api.GET("/query_options", func(c *gin.Context) { c.JSON(http.StatusOK, h.UI) })
}

func (h *Handler) index(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"envs":       h.UI.Envs,
		"namespaces": h.UI.Namespaces,
		"apps":       h.UI.Apps,
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
	res, err := h.queryService().Execute(c.Request.Context(), req)
	if err != nil {
		var validation *query.Error
		if errors.As(err, &validation) {
			c.JSON(validation.Status, gin.H{"error": validation.Message})
			return
		}
		status, body := storageStatusAndBody(err)
		c.JSON(status, gin.H{"error": body})
		return
	}
	c.JSON(http.StatusOK, res)
}
func (h *Handler) queryService() *query.Service {
	namespaces := make([]string, 0, len(h.UI.Namespaces))
	for _, option := range h.UI.Namespaces {
		namespaces = append(namespaces, option.Value)
	}
	return &query.Service{Client: h.Client, Limit: h.Limit, Namespaces: namespaces}
}

const maxQueryRangeBodyBytes int64 = 16 * 1024

// storageStatusAndBody maps storage errors to appropriate HTTP status codes and response bodies.
func storageStatusAndBody(err error) (int, string) {
	var httpErr *storage.HTTPError
	if errors.As(err, &httpErr) {
		// 4xx from storage → same 4xx to client; 5xx → 502
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			return httpErr.StatusCode, "Storage: " + httpErr.Message
		}
	}
	// Network/timeout or other errors → 502
	return http.StatusBadGateway, "Storage: " + err.Error()
}
