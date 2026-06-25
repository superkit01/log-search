package main

import (
	"flag"
	"log/slog"
	"os"

	"log-search/internal/config"
	"log-search/internal/loki"
	"log-search/internal/web"

	"github.com/gin-gonic/gin"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config failed", "err", err)
		os.Exit(1)
	}
	timeout, err := cfg.Loki.Duration()
	if err != nil {
		slog.Error("invalid loki timeout", "err", err)
		os.Exit(1)
	}

	lc := loki.NewHTTPClient(cfg.Loki.URL, timeout)

	r := gin.Default()
	r.LoadHTMLGlob("web/templates/*")
	r.Static("/static", "./web/static")
	web.New(lc, cfg.Loki.DefaultLimit).Register(r)

	slog.Info("starting log-search", "addr", cfg.Server.Addr, "loki", cfg.Loki.URL)
	if err := r.Run(cfg.Server.Addr); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
