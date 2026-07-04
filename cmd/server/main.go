package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"log-search/internal/config"
	"log-search/internal/loki"
	"log-search/internal/web"
	assets "log-search/web"

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
	if err := r.SetTrustedProxies(nil); err != nil {
		slog.Error("set trusted proxies failed", "err", err)
		os.Exit(1)
	}
	if err := web.MountAssets(r, assets.FS); err != nil {
		slog.Error("mount embedded assets failed", "err", err)
		os.Exit(1)
	}
	web.New(lc, cfg.Loki.DefaultLimit, cfg.UI).Register(r)

	slog.Info("starting log-search", "addr", cfg.Server.Addr, "loki", cfg.Loki.URL)
	if err := newHTTPServer(cfg, r).ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func newHTTPServer(cfg *config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}
