package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_ValidConfig(t *testing.T) {
	p := writeTempYAML(t, `
server:
  addr: ":9090"
loki:
  url: "http://loki.example:3100"
  timeout: "45s"
  default_limit: 500
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.Server.Addr != ":9090" {
		t.Errorf("addr = %q, want :9090", c.Server.Addr)
	}
	if c.Loki.URL != "http://loki.example:3100" {
		t.Errorf("url = %q", c.Loki.URL)
	}
	d, err := c.Loki.Duration()
	if err != nil {
		t.Fatal(err)
	}
	if d != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", d)
	}
	if c.Loki.DefaultLimit != 500 {
		t.Errorf("limit = %d, want 500", c.Loki.DefaultLimit)
	}
}

func TestLoad_MissingURLFails(t *testing.T) {
	p := writeTempYAML(t, `
server:
  addr: ":8080"
loki:
  timeout: "30s"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
}

func TestLoad_BadTimeoutFails(t *testing.T) {
	p := writeTempYAML(t, `
loki:
  url: "http://x:3100"
  timeout: "not-a-duration"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for bad timeout, got nil")
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	p := writeTempYAML(t, `
loki:
  url: "http://x:3100"
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Addr != ":8080" {
		t.Errorf("addr default = %q", c.Server.Addr)
	}
	if c.Loki.Timeout != "30s" {
		t.Errorf("timeout default = %q", c.Loki.Timeout)
	}
	if c.Loki.DefaultLimit != 1000 {
		t.Errorf("limit default = %d", c.Loki.DefaultLimit)
	}
}
