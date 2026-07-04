package main

import (
	"net/http"
	"testing"
	"time"

	"log-search/internal/config"

	"github.com/gin-gonic/gin"
)

func TestNewHTTPServerHasTightTimeouts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv := newHTTPServer(&config.Config{Server: config.ServerCfg{Addr: ":0"}}, gin.New())

	if srv.Addr != ":0" {
		t.Fatalf("addr = %q, want :0", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 10*time.Second {
		t.Fatalf("ReadTimeout = %v, want 10s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 60*time.Second {
		t.Fatalf("WriteTimeout = %v, want 60s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Fatalf("IdleTimeout = %v, want 120s", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 1<<20)
	}
	if _, ok := srv.Handler.(*gin.Engine); !ok {
		t.Fatalf("Handler = %T, want *gin.Engine", srv.Handler)
	}
}

var _ *http.Server = newHTTPServer(&config.Config{}, gin.New())
