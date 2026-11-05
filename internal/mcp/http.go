package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log-search/internal/agenttools"
)

type Caller interface {
	Call(context.Context, string, json.RawMessage) (any, error)
}
type request struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Credential struct {
	Name string `yaml:"name"`
	Key  string `yaml:"api-key"`
}

type Config struct {
	Enabled     bool         `yaml:"enabled"`
	Credentials []Credential `yaml:"credentials"`
	ExternalURL string       `yaml:"external_url"`
}

func (c *Config) Validate() error {
	if c.ExternalURL == "" {
		c.ExternalURL = "http://localhost:8080"
	}
	if !c.Enabled {
		return nil
	}
	keys := c.keys()
	if len(keys) == 0 {
		return fmt.Errorf("mcp.credentials is required when MCP is enabled")
	}
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, item := range c.Credentials {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("MCP key names must be nonempty")
		}
		if names[name] {
			return fmt.Errorf("MCP key names must be unique")
		}
		names[name] = true
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n\t ") {
			return fmt.Errorf("MCP key names and values must be nonempty; keys must not contain whitespace")
		}
		if seen[key] {
			return fmt.Errorf("MCP API keys must be unique across users")
		}
		seen[key] = true
	}
	_, err := agenttools.New(c.ExternalURL, 45*time.Second)
	if err != nil {
		return fmt.Errorf("invalid mcp.external_url: %w", err)
	}
	return nil
}

func (c Config) keys() []string {
	keys := make([]string, 0, len(c.Credentials))
	for _, item := range c.Credentials {
		keys = append(keys, item.Key)
	}
	return keys
}

// HTTPHandler implements stateless Streamable HTTP (JSON responses, no SSE stream).
// There are no session IDs: clients may issue initialized requests independently.
func HTTPHandler(cfg Config, tools Caller) http.Handler {
	var expected [][32]byte
	for _, key := range cfg.keys() {
		if strings.TrimSpace(key) != "" && !strings.ContainsAny(key, "\r\n\t ") {
			expected = append(expected, sha256.Sum256([]byte("Bearer "+key)))
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		supplied := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		matched := 0
		for _, hash := range expected {
			matched |= subtle.ConstantTimeCompare(hash[:], supplied[:])
		}
		if !cfg.Enabled || matched != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="log-search-mcp"`)
			http.Error(w, "Unauthorized", 401)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if version := r.Header.Get("MCP-Protocol-Version"); version != "" && version != "2025-06-18" {
			http.Error(w, "Unsupported MCP protocol version", http.StatusBadRequest)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			allowed, _ := url.Parse(cfg.ExternalURL)
			given, err := url.Parse(origin)
			if err != nil || given.Scheme != allowed.Scheme || given.Host != allowed.Host || given.User != nil || (given.Path != "" && given.Path != "/") || given.RawQuery != "" || given.Fragment != "" {
				http.Error(w, "Forbidden origin", 403)
				return
			}
		}
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method not allowed", 405)
			return
		}
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			http.Error(w, "Expected application/json", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request too large", 413)
			return
		}
		var req request
		write := func(result any, code int, message string) {
			response := map[string]any{"jsonrpc": "2.0", "id": req.ID}
			if code != 0 {
				response["error"] = map[string]any{"code": code, "message": message}
			} else {
				response["result"] = result
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
		if json.Unmarshal(data, &req) != nil {
			write(nil, -32700, "Parse error")
			return
		}
		if req.Version != "2.0" || req.Method == "" {
			write(nil, -32600, "Invalid Request")
			return
		}
		if len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var id any
		_ = json.Unmarshal(req.ID, &id)
		switch id.(type) {
		case string, float64:
		default:
			write(nil, -32600, "Invalid request ID")
			return
		}
		switch req.Method {
		case "initialize":
			var p struct {
				Version string `json:"protocolVersion"`
			}
			if json.Unmarshal(req.Params, &p) != nil || p.Version == "" {
				write(nil, -32602, "protocolVersion is required")
				return
			}
			write(map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "log-search", "version": "1.0.0"}}, 0, "")
		case "ping":
			write(map[string]any{}, 0, "")
		case "tools/list":
			write(map[string]any{"tools": agenttools.Tools()}, 0, "")
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if json.Unmarshal(req.Params, &p) != nil || (p.Name != "get_query_options" && p.Name != "query_logs") {
				write(nil, -32602, "Invalid tool name or parameters")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			defer cancel()
			value, err := tools.Call(ctx, p.Name, p.Arguments)
			if err != nil {
				write(map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": err.Error()}}}, 0, "")
				return
			}
			text, _ := json.Marshal(value)
			write(map[string]any{"isError": false, "structuredContent": value, "content": []any{map[string]any{"type": "text", "text": string(text)}}}, 0, "")
		default:
			write(nil, -32601, "Method not found")
		}
	})
}
