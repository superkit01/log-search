package main

import (
	"flag"
	"log/slog"
	"os"

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
	if err := web.MountAssets(r, assets.FS); err != nil {
		slog.Error("mount embedded assets failed", "err", err)
		os.Exit(1)
	}
	ui := web.UIOptions{Apps: cfg.UI.Apps}
	for _, env := range cfg.UI.Envs {
		ui.Envs = append(ui.Envs, web.Choice{Label: env.Label, Value: env.Value})
	}
	web.New(lc, cfg.Loki.DefaultLimit, ui).Register(r)

	slog.Info("starting log-search", "addr", cfg.Server.Addr, "loki", cfg.Loki.URL)
	if err := r.Run(cfg.Server.Addr); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
