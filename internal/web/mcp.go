package web

import (
	"github.com/gin-gonic/gin"
	"log-search/internal/agenttools"
	"log-search/internal/mcp"
	"time"
)

func (h *Handler) RegisterMCP(r *gin.Engine, cfg mcp.Config) {
	h.mcpConfig = cfg
	if !cfg.Enabled {
		return
	}
	adapter, err := agenttools.New(cfg.ExternalURL, 45*time.Second)
	if err != nil {
		panic(err)
	} // Config is validated at startup.
	adapter.Execute = h.queryService().Execute
	adapter.Options = h.UI
	r.Any("/mcp", gin.WrapH(mcp.HTTPHandler(cfg, adapter)))
}
