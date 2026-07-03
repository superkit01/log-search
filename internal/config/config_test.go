package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestLoad_UIOptions(t *testing.T) {
	p := writeTempYAML(t, `
loki:
  url: "http://loki.example:3100"
ui:
  envs:
    - label: 预发
      value: staging
    - label: 生产
      value: prod
  apps:
    - boeet-alarm
    - boeet-cms
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(c.UI.Envs) != 2 || c.UI.Envs[0].Label != "预发" || c.UI.Envs[0].Value != "staging" {
		t.Fatalf("envs = %+v", c.UI.Envs)
	}
	if len(c.UI.Apps) != 2 || c.UI.Apps[1] != "boeet-cms" {
		t.Fatalf("apps = %+v", c.UI.Apps)
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

func TestLoad_EnvExpansion_Override(t *testing.T) {
	// ${LOKI_URL:-default}:env 设了且非空时取 env
	t.Setenv("LOKI_URL", "http://from-env:3100")
	p := writeTempYAML(t, `
loki:
  url: ${LOKI_URL:-http://default:3100}
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.Loki.URL != "http://from-env:3100" {
		t.Errorf("url = %q, want env override http://from-env:3100", c.Loki.URL)
	}
}

func TestLoad_EnvExpansion_DefaultWhenUnset(t *testing.T) {
	// env 未设/为空时回落到 :- 后的默认值
	t.Setenv("LOKI_URL", "")
	p := writeTempYAML(t, `
loki:
  url: ${LOKI_URL:-http://default:3100}
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.Loki.URL != "http://default:3100" {
		t.Errorf("url = %q, want default http://default:3100", c.Loki.URL)
	}
}

func TestLoad_EnvExpansion_StillValidated(t *testing.T) {
	// 展开后的值仍走 validate():非法值报错
	t.Setenv("LOKI_URL", "not-a-url")
	p := writeTempYAML(t, `
loki:
  url: ${LOKI_URL:-http://default:3100}
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error for invalid LOKI_URL env, got nil")
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
	if len(c.UI.Envs) != 1 || c.UI.Envs[0].Label != "演示" || c.UI.Envs[0].Value != "demo" {
		t.Errorf("env defaults = %+v", c.UI.Envs)
	}
}

func TestLoad_URL_MalformedFails(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid https URL",
			url:     "https://loki.example:3100",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			url:     "http://loki.example:3100",
			wantErr: false,
		},
		{
			name:    "malformed - not a URL",
			url:     "not-a-url",
			wantErr: true,
			errMsg:  "loki.url invalid",
		},
		{
			name:    "wrong scheme - gopher",
			url:     "gopher://loki.example",
			wantErr: true,
			errMsg:  "loki.url invalid",
		},
		{
			name:    "wrong scheme - ftp",
			url:     "ftp://loki.example",
			wantErr: true,
			errMsg:  "loki.url invalid",
		},
		{
			name:    "missing host",
			url:     "http://",
			wantErr: true,
			errMsg:  "loki.url invalid",
		},
		{
			name:    "missing scheme",
			url:     "loki.example:3100",
			wantErr: true,
			errMsg:  "loki.url invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTempYAML(t, fmt.Sprintf(`
loki:
  url: %q
`, tt.url))
			_, err := Load(p)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errMsg)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error should contain %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}
