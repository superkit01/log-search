package mcp

import (
	"context"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeTools struct{}

func TestAPIKeyListConfiguration(t *testing.T) {
	var cfg Config
	data := "enabled: true\ncredentials:\n  - name: alice\n    api-key: alice-key\n  - name: bob\n    api-key: bob-key\n"
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Credentials) != 2 || cfg.Credentials[0].Name != "alice" || cfg.Credentials[0].Key != "alice-key" || cfg.Credentials[1].Name != "bob" {
		t.Fatal("credential list not decoded correctly")
	}
	if err := yaml.Unmarshal([]byte("enabled: true\ncredentials:\n  alice: old-key\n"), &Config{}); err == nil {
		t.Fatal("old key map accepted")
	}
}

func TestSingleAPIKeyConfigurationIsNotAccepted(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("enabled: true\napi_key: removed-key\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Validate() == nil {
		t.Fatal("legacy single key still accepted")
	}
	cfg.Credentials = []Credential{{Name: "default", Key: "current-key"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	r.Header.Set("Authorization", "Bearer removed-key")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HTTPHandler(cfg, &fakeTools{}).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("removed key authorized a request")
	}
}

func (*fakeTools) Call(context.Context, string, json.RawMessage) (any, error) {
	return map[string]any{"envs": []string{"test"}}, nil
}

func TestHTTPBearerAndProtocol(t *testing.T) {
	cfg := Config{Enabled: true, Credentials: []Credential{{Name: "default", Key: "test-key"}}, ExternalURL: "http://localhost:8080"}
	handler := HTTPHandler(cfg, &fakeTools{})
	for _, tc := range []struct {
		method, auth, origin, body string
		status                     int
	}{
		{"POST", "", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 401},
		{"POST", "Bearer wrong", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 401},
		{"POST", "Basic test-key", "", `{}`, 401},
		{"GET", "Bearer test-key", "", "", 405},
		{"POST", "Bearer test-key", "https://evil.example", `{}`, 403},
		{"POST", "Bearer test-key", "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, 200},
		{"POST", "Bearer test-key", "http://localhost:8080", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, 200},
		{"POST", "Bearer test-key", "", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_query_options","arguments":{}}}`, 200},
		{"POST", "Bearer test-key", "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, 202},
	} {
		r := httptest.NewRequest(tc.method, "/mcp", strings.NewReader(tc.body))
		r.Header.Set("Authorization", tc.auth)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s auth=%q status=%d body=%s", tc.method, tc.auth, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "test-key") {
			t.Fatal("key leaked")
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer test-key")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HTTPHandler(Config{Enabled: true}, &fakeTools{}).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("empty key allowed")
	}
}

func TestMCPConfigValidation(t *testing.T) {
	for _, key := range []string{"", " ", "a\nb", "a b"} {
		c := Config{Enabled: true, Credentials: []Credential{{Name: "default", Key: key}}}
		if c.Validate() == nil {
			t.Fatal("accepted invalid key")
		}
	}
	c := Config{Enabled: true, Credentials: []Credential{{Name: "default", Key: "valid"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c = Config{Enabled: true, Credentials: []Credential{{Name: "default", Key: "valid"}}, ExternalURL: "http://user:secret@host"}
	if c.Validate() == nil {
		t.Fatal("credentials in public URL allowed")
	}
}

func TestMultipleUserKeysAndRevocation(t *testing.T) {
	cfg := Config{Enabled: true, Credentials: []Credential{{Name: "alice", Key: "alice-key"}, {Name: "bob", Key: "bob-key"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	check := func(h http.Handler, key string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("unexpected authorization status %d, want %d", w.Code, want)
		}
	}
	h := HTTPHandler(cfg, &fakeTools{})
	for _, key := range []string{"alice-key", "bob-key"} {
		check(h, key, 200)
	}
	check(h, "unknown", 401)
	cfg.Credentials = cfg.Credentials[1:]
	h = HTTPHandler(cfg, &fakeTools{})
	check(h, "alice-key", 401)
	check(h, "bob-key", 200)
}

func TestInvalidUserKeyConfiguration(t *testing.T) {
	for _, keys := range [][]Credential{
		{{Name: "alice", Key: ""}}, {{Name: "", Key: "valid"}}, {{Name: "alice", Key: "a b"}},
		{{Name: "alice", Key: "same"}, {Name: "bob", Key: "same"}},
		{{Name: "alice", Key: "key1"}, {Name: "alice", Key: "key2"}},
		{{Name: "alice", Key: "key1"}, {Name: " alice ", Key: "key2"}},
	} {
		cfg := Config{Enabled: true, Credentials: keys}
		if cfg.Validate() == nil {
			t.Fatal("accepted invalid user keys")
		}
	}
}
