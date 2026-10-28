package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"log-search/internal/config"
	"log-search/internal/elasticsearch"
	"log-search/internal/loki"
	"log-search/internal/storage"
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

	// 根据配置创建存储客户端
	var client storage.Client
	var limit int

	switch cfg.Storage.Type {
	case "es", "elasticsearch":
		timeout, err := cfg.Storage.ES.Duration()
		if err != nil {
			slog.Error("invalid elasticsearch timeout", "err", err)
			os.Exit(1)
		}
		client = elasticsearch.NewHTTPClient(cfg.Storage.ES.URL, cfg.Storage.ES.Index, timeout, elasticsearch.Options{Fields: cfg.Storage.ES.Fields, IndexTimezone: cfg.Storage.ES.IndexTimezone})
		limit = cfg.Storage.ES.DefaultLimit
		slog.Info("using Elasticsearch storage", "url", cfg.Storage.ES.URL, "index", cfg.Storage.ES.Index)
	default: // loki
		timeout, err := cfg.Storage.Loki.Duration()
		if err != nil {
			slog.Error("invalid loki timeout", "err", err)
			os.Exit(1)
		}
		client = loki.NewHTTPClient(cfg.Storage.Loki.URL, timeout)
		limit = cfg.Storage.Loki.DefaultLimit
		slog.Info("using Loki storage", "url", cfg.Storage.Loki.URL)
	}

	r := gin.Default()
	if err := r.SetTrustedProxies(nil); err != nil {
		slog.Error("set trusted proxies failed", "err", err)
		os.Exit(1)
	}
	if err := web.MountAssets(r, assets.FS); err != nil {
		slog.Error("mount embedded assets failed", "err", err)
		os.Exit(1)
	}
	web.New(client, limit, cfg.UI).Register(r)

	slog.Info("starting log-search", "addr", cfg.Server.Addr)
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
