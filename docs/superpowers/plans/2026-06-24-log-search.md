# 日志查询工程 (Log Search) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个前后端不分离的日志查询 Web 工程(Go + Gin 渲染页面),从已有 Loki 实例用 LogQL 查询日志,展示按级别着色的结果列表和结果直方图。

**Architecture:** Gin 单进程渲染 HTML 模板 + 暴露 JSON API;后端用标准库 `net/http` 直连 Loki HTTP API(方案 A);级别解析为纯函数,handler 通过 `loki.Client` 接口注入便于测试。

**Tech Stack:** Go 1.24、Gin v1.10、gopkg.in/yaml.v3、`net/http`、`log/slog`、原生 JS + Chart.js(CDN)。

## Global Constraints

(每个任务的隐含要求,逐字取自 spec)

- Go 版本:1.24;模块路径:`log-search`
- 框架:`github.com/gin-gonic/gin v1.10`;配置:`gopkg.in/yaml.v3`;其余仅用标准库
- 与 Loki 通信:**直接 HTTP**(`/loki/api/v1/query_range`、`/labels`、`/label/{name}/values`),不引入 loki-client-go
- 前端:Gin HTML 模板 + 原生 JS,**无构建步骤**;直方图用 CDN Chart.js
- **无鉴权**(内网工具)
- `/api/query_range` 参数:`query`(必填)、`start`/`end`(unix 秒)、`limit`(默认 1000,上限 **5000**)、`direction`(FORWARD/BACKWARD,默认 BACKWARD)
- 错误码:Loki 不可达/超时→**502**;LogQL 语法错/参数非法→**400**(透传 Loki 错误信息);配置缺失→**启动失败**
- 级别规则:JSON `"level"/"levelname"`、kv `level=/lvl=/severity=`、文本 `[ERROR]/ERROR/WARN/INFO/DEBUG`;ERROR→红、WARN→黄、INFO→蓝、DEBUG→灰、无匹配→默认 INFO
- 配置文件 `config.yaml`(flag `--config` 指定,默认 `config.yaml`),含 `server.addr`、`loki.url`、`loki.timeout`、`loki.default_limit`
- 简化说明:模板使用**单个** `index.html`(不做 layout 拆分,spec 列了两个文件,此处按 YAGNI 合并为一个,功能等价)

---

## File Structure

| 文件 | 职责 |
|---|---|
| `go.mod` / `go.sum` | 模块定义与依赖锁定 |
| `config.example.yaml` | 配置模板 |
| `Makefile` / `README.md` | 构建脚本与说明 |
| `cmd/server/main.go` | 入口:加载配置→组装依赖→启动 Gin |
| `internal/config/config.go` | 配置结构 + Load + 校验/默认值 |
| `internal/config/config_test.go` | 配置加载测试 |
| `internal/logparse/level.go` | 级别解析纯函数 + CSSClass |
| `internal/logparse/level_test.go` | 表驱动测试 |
| `internal/loki/types.go` | Line/QueryResult/Direction + 内部 Loki JSON 结构 |
| `internal/loki/client.go` | Client 接口 + httpClient 实现(Labels/LabelValues/QueryRange) |
| `internal/loki/client_test.go` | httptest mock 测试 |
| `internal/web/handler.go` | 路由 + handler(含参数校验、级别填充) |
| `internal/web/handler_test.go` | 注入 mock loki.Client 测试 |
| `web/templates/index.html` | 查询页面 |
| `web/static/app.js` | 前端逻辑(取标签/查询/渲染/直方图) |
| `web/static/style.css` | 暗色主题与级别配色 |

---

## Task 1: 工程脚手架 + config 包

**Files:**
- Create: `go.mod`, `config.example.yaml`, `Makefile`, `README.md`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load(path string) (*config.Config, error)`;`Config` 含 `Server.Addr string`、`Loki.URL string`、`Loki.Timeout string`、`Loki.DefaultLimit int`;方法 `LokiCfg.Duration() (time.Duration, error)`

- [ ] **Step 1: 初始化模块与目录**

Run:
```bash
go mod init log-search
mkdir -p internal/config internal/logparse internal/loki internal/web cmd/server web/templates web/static
```
Expected: 生成 `go.mod`(`module log-search`),目录创建成功。

- [ ] **Step 2: 写失败测试 `internal/config/config_test.go`**

```go
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
```

- [ ] **Step 3: 运行测试,确认失败**

Run: `go test ./internal/config/`
Expected: FAIL —— 编译错误 `undefined: Load`(尚未实现)。

- [ ] **Step 4: 实现 `internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerCfg `yaml:"server"`
	Loki   LokiCfg   `yaml:"loki"`
}

type ServerCfg struct {
	Addr string `yaml:"addr"`
}

type LokiCfg struct {
	URL          string `yaml:"url"`
	Timeout      string `yaml:"timeout"`
	DefaultLimit int    `yaml:"default_limit"`
}

// Duration 解析 timeout 字符串为 time.Duration。
func (l LokiCfg) Duration() (time.Duration, error) {
	return time.ParseDuration(l.Timeout)
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.Loki.URL == "" {
		return fmt.Errorf("loki.url is required")
	}
	if c.Loki.Timeout == "" {
		c.Loki.Timeout = "30s"
	}
	if _, err := time.ParseDuration(c.Loki.Timeout); err != nil {
		return fmt.Errorf("loki.timeout invalid %q: %w", c.Loki.Timeout, err)
	}
	if c.Loki.DefaultLimit <= 0 {
		c.Loki.DefaultLimit = 1000
	}
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	return nil
}
```

- [ ] **Step 5: 拉取依赖并运行测试,确认通过**

Run:
```bash
go mod tidy
go test ./internal/config/ -v
```
Expected: PASS(全部 4 个测试)。`go.sum` 生成。

- [ ] **Step 6: 写 `config.example.yaml`、`Makefile`、`README.md`**

`config.example.yaml`:
```yaml
server:
  addr: ":8080"
loki:
  url: "http://localhost:3100"   # 用户的 Loki 地址,请改成实际地址
  timeout: "30s"                 # HTTP 客户端超时
  default_limit: 1000            # 查询默认条数
```

`Makefile`:
```makefile
.PHONY: run build test tidy vet
run:
	go run ./cmd/server
build:
	mkdir -p bin && go build -o bin/log-search ./cmd/server
test:
	go test ./...
vet:
	go vet ./...
tidy:
	go mod tidy
```

`README.md`:
```markdown
# Log Search

基于 Gin + Loki 的日志查询工程(前后端不分离)。

## 快速开始

1. 复制配置并填入你的 Loki 地址:
   `cp config.example.yaml config.yaml`
2. 运行:`make run`(或 `go run ./cmd/server`)
3. 浏览器打开 http://localhost:8080

## 开发

- 测试:`make test`
- 构建:`make build`
```

- [ ] **Step 7: 提交**

```bash
git add go.mod go.sum config.example.yaml Makefile README.md internal/config
git commit -m "feat(config): add config loading with validation and defaults"
```

---

## Task 2: logparse 级别解析(纯函数)

**Files:**
- Create: `internal/logparse/level.go`
- Test: `internal/logparse/level_test.go`

**Interfaces:**
- Produces: `logparse.Level` 类型(常量 `LevelError/LevelWarn/LevelInfo/LevelDebug`,均为小写字符串如 `"error"`);`logparse.DetectLevel(line string) Level`;`Level.CSSClass() string`(返回 `lvl-error` 等)

- [ ] **Step 1: 写失败测试 `internal/logparse/level_test.go`**

```go
package logparse

import "testing"

func TestDetectLevel(t *testing.T) {
	cases := []struct {
		in   string
		want Level
	}{
		{`{"level":"error","msg":"x"}`, LevelError},
		{`{"levelname":"WARN","msg":"x"}`, LevelWarn},
		{`{"level":"info"}`, LevelInfo},
		{`{"level":"debug"}`, LevelDebug},
		{`level=error msg=x`, LevelError},
		{`lvl=warn msg=x`, LevelWarn},
		{`severity=info x`, LevelInfo},
		{`[ERROR] connection refused`, LevelError},
		{`2026-06-24 INFO started`, LevelInfo},
		{`FATAL panic`, LevelError},
		{`something went wrong here`, LevelInfo}, // 无匹配 → 默认 INFO
		{`{"msg":"ok","level":"trace"}`, LevelDebug},
	}
	for _, c := range cases {
		got := DetectLevel(c.in)
		if got != c.want {
			t.Errorf("DetectLevel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCSSClass(t *testing.T) {
	cases := map[Level]string{
		LevelError: "lvl-error",
		LevelWarn:  "lvl-warn",
		LevelInfo:  "lvl-info",
		LevelDebug: "lvl-debug",
	}
	for lv, want := range cases {
		if got := lv.CSSClass(); got != want {
			t.Errorf("%v CSSClass = %q, want %q", lv, got, want)
		}
	}
}
```

- [ ] **Step 2: 运行测试,确认失败**

Run: `go test ./internal/logparse/`
Expected: FAIL —— `undefined: DetectLevel`、`undefined: Level`。

- [ ] **Step 3: 实现 `internal/logparse/level.go`**

```go
package logparse

import (
	"regexp"
	"strings"
)

// Level 表示日志级别。值为小写字符串(与前端 CSS class 后缀一致)。
type Level string

const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
	LevelDebug Level = "debug"
)

// CSSClass 返回前端着色用的 class 名。
func (l Level) CSSClass() string {
	switch l {
	case LevelError:
		return "lvl-error"
	case LevelWarn:
		return "lvl-warn"
	case LevelDebug:
		return "lvl-debug"
	default:
		return "lvl-info"
	}
}

var (
	// 结构化: "level":"error" / level=error / lvl=x / severity=x
	structuredRe = regexp.MustCompile(`(?i)(?:level|levelname|lvl|severity)["']?\s*[:=]\s*["']?([a-z]+)`)
	// 文本: [ERROR] / ERROR / WARN / INFO / DEBUG
	textRe = regexp.MustCompile(`(?i)\b(ERROR|ERR|FATAL|PANIC|WARN(?:ING)?|INFO|DEBUG|TRACE)\b`)
)

// DetectLevel 从单行日志文本中解析级别,无匹配返回 LevelInfo。
func DetectLevel(line string) Level {
	if s := structuredRe.FindStringSubmatch(line); s != nil {
		if lv := normalize(s[1]); lv != "" {
			return lv
		}
	}
	if t := textRe.FindStringSubmatch(line); t != nil {
		if lv := normalize(t[1]); lv != "" {
			return lv
		}
	}
	return LevelInfo
}

// normalize 把捕获到的词归一化为 Level;非级别词返回空字符串。
func normalize(s string) Level {
	switch strings.ToLower(s) {
	case "error", "err", "fatal", "panic":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "info", "information":
		return LevelInfo
	case "debug", "trace":
		return LevelDebug
	}
	return ""
}
```

- [ ] **Step 4: 运行测试,确认通过**

Run: `go test ./internal/logparse/ -v`
Expected: PASS(2 个测试,13 个级别用例全过)。

- [ ] **Step 5: 提交**

```bash
git add internal/logparse
git commit -m "feat(logparse): add log level detection with CSS class mapping"
```

---

## Task 3: loki 客户端(类型 + HTTP 实现)

**Files:**
- Create: `internal/loki/types.go`
- Create: `internal/loki/client.go`
- Test: `internal/loki/client_test.go`

**Interfaces:**
- Produces:
  - `loki.Line{Time time.Time, Level string, Raw string, Labels map[string]string}`
  - `loki.QueryResult{Lines []Line, Total int}`
  - `loki.Direction`(`loki.Forward="FORWARD"`、`loki.Backward="BACKWARD"`)
  - `loki.Client` 接口:`Labels(ctx, start, end) ([]string, error)`、`LabelValues(ctx, name, start, end) ([]string, error)`、`QueryRange(ctx, query, start, end, limit, direction) (*QueryResult, error)`
  - `loki.NewHTTPClient(url string, timeout time.Duration) Client`
- 注:客户端返回的 `Line.Level` 为空字符串,级别由 web 层填充。

- [ ] **Step 1: 写失败测试 `internal/loki/client_test.go`**

```go
package loki

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL, 5*time.Second)
}

func TestQueryRange_ParsesStreams(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		fmt.Fprint(w, `{
		  "status":"success",
		  "data":[{
		    "stream":{"app":"nginx"},
		    "values":[["1609459200000000000","level=info started"],["1609459201000000000","level=error boom"]]
		  }]
		}`)
	})
	res, err := c.QueryRange(context.Background(), `{app="nginx"}`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d want 2", res.Total)
	}
	if res.Lines[0].Raw != "level=info started" {
		t.Errorf("line0 raw = %q", res.Lines[0].Raw)
	}
	if res.Lines[0].Labels["app"] != "nginx" {
		t.Errorf("labels = %v", res.Lines[0].Labels)
	}
	if res.Lines[0].Level != "" {
		t.Errorf("client must not set level, got %q", res.Lines[0].Level)
	}
}

func TestLabels(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/labels" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"success","data":["app","env"]}`)
	})
	got, err := c.Labels(context.Background(), time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "app" {
		t.Errorf("labels = %v", got)
	}
}

func TestLabelValues(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/app/values" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"success","data":["nginx","api"]}`)
	})
	got, err := c.LabelValues(context.Background(), "app", time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("values = %v", got)
	}
}

func TestQueryRange_LokiError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"parse error at col 1"}`)
	})
	_, err := c.QueryRange(context.Background(), `{`, time.Unix(0, 0), time.Unix(0, 0).Add(time.Hour), 100, Backward)
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: 运行测试,确认失败**

Run: `go test ./internal/loki/`
Expected: FAIL —— `undefined: Client`、`undefined: NewHTTPClient` 等。

- [ ] **Step 3: 实现 `internal/loki/types.go`**

```go
package loki

import (
	"encoding/json"
	"time"
)

// Line 是拍平后的单条日志。Level 由上层(web)填充。
type Line struct {
	Time   time.Time         `json:"ts"`
	Level  string            `json:"level"`
	Raw    string            `json:"line"`
	Labels map[string]string `json:"labels"`
}

// QueryResult 是范围查询的聚合结果。
type QueryResult struct {
	Lines []Line `json:"lines"`
	Total int    `json:"total"`
}

// Direction 是查询方向。
type Direction string

const (
	Forward  Direction = "FORWARD"
	Backward Direction = "BACKWARD"
)

// 以下是 Loki JSON 响应的内部结构(不导出)。

type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	ErrorMsg  string          `json:"error,omitempty"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"` // [unixNanoString, line]
}
```

- [ ] **Step 4: 实现 `internal/loki/client.go`**

```go
package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client 抽象 Loki 查询能力,便于测试注入 mock。
type Client interface {
	Labels(ctx context.Context, start, end time.Time) ([]string, error)
	LabelValues(ctx context.Context, name string, start, end time.Time) ([]string, error)
	QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction Direction) (*QueryResult, error)
}

type httpClient struct {
	baseURL string
	h       *http.Client
}

// NewHTTPClient 创建直连 Loki HTTP API 的客户端。
func NewHTTPClient(rawURL string, timeout time.Duration) Client {
	return &httpClient{
		baseURL: strings.TrimRight(rawURL, "/"),
		h:       &http.Client{Timeout: timeout},
	}
}

func (c *httpClient) Labels(ctx context.Context, start, end time.Time) ([]string, error) {
	var out []string
	err := c.get(ctx, "/loki/api/v1/labels", url.Values{
		"start": {strconv.FormatInt(start.UnixNano(), 10)},
		"end":   {strconv.FormatInt(end.UnixNano(), 10)},
	}, &out)
	return out, err
}

func (c *httpClient) LabelValues(ctx context.Context, name string, start, end time.Time) ([]string, error) {
	var out []string
	path := fmt.Sprintf("/loki/api/v1/label/%s/values", url.PathEscape(name))
	err := c.get(ctx, path, url.Values{
		"start": {strconv.FormatInt(start.UnixNano(), 10)},
		"end":   {strconv.FormatInt(end.UnixNano(), 10)},
	}, &out)
	return out, err
}

func (c *httpClient) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction Direction) (*QueryResult, error) {
	var streams []stream
	err := c.get(ctx, "/loki/api/v1/query_range", url.Values{
		"query":     {query},
		"start":     {strconv.FormatInt(start.UnixNano(), 10)},
		"end":       {strconv.FormatInt(end.UnixNano(), 10)},
		"limit":     {strconv.Itoa(limit)},
		"direction": {string(direction)},
	}, &streams)
	if err != nil {
		return nil, err
	}
	res := &QueryResult{}
	for _, s := range streams {
		for _, v := range s.Values {
			ns, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				continue
			}
			res.Lines = append(res.Lines, Line{
				Time:   time.Unix(0, ns).UTC(),
				Raw:    v[1],
				Labels: s.Stream,
			})
		}
	}
	res.Total = len(res.Lines)
	return res, nil
}

func (c *httpClient) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if enc := params.Encode(); enc != "" {
		u += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.h.Do(req)
	if err != nil {
		return fmt.Errorf("loki request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var ar apiResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return fmt.Errorf("loki: decode %s: %w (body=%q)", path, err, string(body))
	}
	if ar.Status != "success" {
		return fmt.Errorf("loki: %s: %s", ar.ErrorType, ar.ErrorMsg)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("loki: HTTP %d: %s", resp.StatusCode, ar.ErrorMsg)
	}
	if out != nil {
		if err := json.Unmarshal(ar.Data, out); err != nil {
			return fmt.Errorf("loki: decode data %s: %w", path, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: 运行测试,确认通过**

Run: `go test ./internal/loki/ -v`
Expected: PASS(4 个测试)。

- [ ] **Step 6: 提交**

```bash
git add internal/loki
git commit -m "feat(loki): add direct HTTP client for labels/label-values/query_range"
```

---

## Task 4: web handler(路由 + 参数校验 + 级别填充)

**Files:**
- Create: `internal/web/handler.go`
- Test: `internal/web/handler_test.go`

**Interfaces:**
- Consumes: `loki.Client`(Task 3)、`logparse.DetectLevel`(Task 2)
- Produces: `web.New(lc loki.Client, defaultLimit int) *Handler`;`(*Handler).Register(r *gin.Engine)` 注册路由 `GET /`、`GET /api/labels`、`GET /api/label/:name/values`、`GET /api/query_range`

- [ ] **Step 1: 添加 gin 依赖**

Run:
```bash
go get github.com/gin-gonic/gin@v1.10.0
go mod tidy
```
Expected: `go.mod` 出现 gin 依赖,`go.sum` 更新。

- [ ] **Step 2: 写失败测试 `internal/web/handler_test.go`**

```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log-search/internal/loki"

	"github.com/gin-gonic/gin"
)

type mockLoki struct {
	labels      []string
	labelValues []string
	result      *loki.QueryResult
	err         error
}

func (m *mockLoki) Labels(ctx context.Context, start, end time.Time) ([]string, error) {
	return m.labels, m.err
}
func (m *mockLoki) LabelValues(ctx context.Context, name string, start, end time.Time) ([]string, error) {
	return m.labelValues, m.err
}
func (m *mockLoki) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction loki.Direction) (*loki.QueryResult, error) {
	return m.result, m.err
}

func setupRouter(m loki.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	New(m, 1000).Register(r)
	return r
}

func TestQueryRange_OK_FillsLevel(t *testing.T) {
	m := &mockLoki{result: &loki.QueryResult{
		Lines: []loki.Line{{Raw: "level=error boom"}},
		Total: 1,
	}}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=foo&start=1&end=99999&limit=10&direction=FORWARD", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var res loki.QueryResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 1 || res.Lines[0].Level != "error" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestQueryRange_MissingQuery(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?start=1&end=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_BadLimit(t *testing.T) {
	r := setupRouter(&mockLoki{})
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999&limit=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestQueryRange_LokiError_502(t *testing.T) {
	m := &mockLoki{err: context.DeadlineExceeded}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/query_range?query=x&start=1&end=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Loki") {
		t.Errorf("body should mention Loki: %s", w.Body.String())
	}
}

func TestLabels_OK(t *testing.T) {
	m := &mockLoki{labels: []string{"app", "env"}}
	r := setupRouter(m)
	req := httptest.NewRequest(http.MethodGet, "/api/labels?start=1&end=99999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		Data []string `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Data) != 2 {
		t.Errorf("data = %v", got.Data)
	}
}
```

- [ ] **Step 3: 运行测试,确认失败**

Run: `go test ./internal/web/`
Expected: FAIL —— `undefined: New`、`undefined: Handler.Register`。

- [ ] **Step 4: 实现 `internal/web/handler.go`**

```go
package web

import (
	"net/http"
	"strconv"
	"time"

	"log-search/internal/logparse"
	"log-search/internal/loki"

	"github.com/gin-gonic/gin"
)

const maxLimit = 5000

// Handler 持有 Loki 客户端与默认 limit。
type Handler struct {
	Loki  loki.Client
	Limit int
}

// New 创建 Handler。
func New(lc loki.Client, defaultLimit int) *Handler {
	return &Handler{Loki: lc, Limit: defaultLimit}
}

// Register 在 gin engine 上注册路由。
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/", h.index)
	api := r.Group("/api")
	api.GET("/labels", h.labels)
	api.GET("/label/:name/values", h.labelValues)
	api.GET("/query_range", h.queryRange)
}

func (h *Handler) index(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{"defaultLimit": h.Limit})
}

func (h *Handler) labels(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		return
	}
	labels, err := h.Loki.Labels(c.Request.Context(), start, end)
	respondData(c, labels, err)
}

func (h *Handler) labelValues(c *gin.Context) {
	start, end, ok := parseRange(c)
	if !ok {
		return
	}
	vals, err := h.Loki.LabelValues(c.Request.Context(), c.Param("name"), start, end)
	respondData(c, vals, err)
}

func (h *Handler) queryRange(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	start, end, ok := parseRange(c)
	if !ok {
		return
	}
	limit := h.Limit
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and " + strconv.Itoa(maxLimit)})
			return
		}
		limit = n
	}
	direction := loki.Backward
	if c.Query("direction") == string(loki.Forward) {
		direction = loki.Forward
	}
	res, err := h.Loki.QueryRange(c.Request.Context(), query, start, end, limit, direction)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Loki: " + err.Error()})
		return
	}
	for i := range res.Lines {
		res.Lines[i].Level = string(logparse.DetectLevel(res.Lines[i].Raw))
	}
	c.JSON(http.StatusOK, res)
}

// parseRange 解析 start/end(unix 秒),返回 false 表示已写入错误响应。
func parseRange(c *gin.Context) (time.Time, time.Time, bool) {
	start, err1 := strconv.ParseInt(c.Query("start"), 10, 64)
	end, err2 := strconv.ParseInt(c.Query("end"), 10, 64)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start and end (unix seconds) are required"})
		return time.Time{}, time.Time{}, false
	}
	st := time.Unix(start, 0)
	en := time.Unix(end, 0)
	if !en.After(st) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end must be after start"})
		return time.Time{}, time.Time{}, false
	}
	return st, en, true
}

// respondData 统一处理 labels/values 的成功/失败响应。
func respondData(c *gin.Context, data interface{}, err error) {
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Loki: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
```

- [ ] **Step 5: 运行测试,确认通过**

Run: `go test ./internal/web/ -v`
Expected: PASS(5 个测试)。

- [ ] **Step 6: 提交**

```bash
git add internal/web go.mod go.sum
git commit -m "feat(web): add gin handler with query_range/labels routes and level fill"
```

---

## Task 5: 前端模板与静态资源

**Files:**
- Create: `web/templates/index.html`
- Create: `web/static/app.js`
- Create: `web/static/style.css`

**Interfaces:**
- Consumes: `GET /`(渲染 `index.html`,注入 `defaultLimit`)、`GET /api/labels`、`GET /api/label/:name/values`、`GET /api/query_range`
- 响应约定:`/api/labels` 与 `/api/label/.../values` 返回 `{"data": [...]}`;`/api/query_range` 返回 `{"lines":[{ts,level,line,labels}],"total":N}`

说明:本任务无 Go 单元测试,验证手段是「构建通过 + main 接线后手动/冒烟」。代码完整给出。

- [ ] **Step 1: 写 `web/templates/index.html`**

```html
<!DOCTYPE html>
<html lang="zh">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>日志查询 - Log Search</title>
  <link rel="stylesheet" href="/static/style.css">
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js"></script>
</head>
<body>
  <header><h1>日志查询</h1></header>
  <main>
    <section class="controls">
      <div class="row">
        <label>标签
          <select id="label-select"><option value="">-- 选择标签 --</option></select>
        </label>
        <label>标签值
          <select id="value-select" disabled><option value="">-- 先选标签 --</option></select>
        </label>
        <button type="button" id="add-label">加入查询</button>
      </div>
      <div class="row">
        <label>关键词(追加 |= "keyword")<input type="text" id="keyword" placeholder="输入后回车"></label>
      </div>
      <div class="row">
        <label>LogQL <input type="text" id="query" class="logql" placeholder='{app="nginx"}'></label>
        <button type="button" id="search">查询</button>
      </div>
      <div class="row">
        <span class="range-group">快捷:
          <button type="button" data-mins="15">15m</button>
          <button type="button" data-mins="60">1h</button>
          <button type="button" data-mins="360">6h</button>
          <button type="button" data-mins="1440">24h</button>
        </span>
        <label>起 <input type="datetime-local" id="start"></label>
        <label>止 <input type="datetime-local" id="end"></label>
        <label>方向
          <select id="direction"><option value="BACKWARD">新→旧</option><option value="FORWARD">旧→新</option></select>
        </label>
        <label>条数 <input type="number" id="limit" value="{{.defaultLimit}}" min="1" max="5000"></label>
      </div>
    </section>

    <section id="error-bar" class="error" hidden></section>
    <section class="chart-wrap"><canvas id="chart" height="80"></canvas></section>
    <section id="results" class="results"></section>
  </main>
  <script src="/static/app.js"></script>
</body>
</html>
```

- [ ] **Step 2: 写 `web/static/style.css`**

```css
* { box-sizing: border-box; }
body { margin: 0; font-family: system-ui, -apple-system, sans-serif; background: #1e1e1e; color: #ddd; }
header { padding: 12px 20px; background: #111; border-bottom: 1px solid #333; }
h1 { margin: 0; font-size: 18px; }
main { padding: 16px 20px; }
.controls .row { display: flex; gap: 12px; align-items: flex-end; margin-bottom: 10px; flex-wrap: wrap; }
label { display: flex; flex-direction: column; font-size: 12px; gap: 4px; color: #aaa; }
input, select { background: #2a2a2a; color: #ddd; border: 1px solid #444; padding: 6px 8px; border-radius: 4px; font-size: 13px; }
.logql { min-width: 480px; }
button { background: #3a7bd5; color: #fff; border: 0; padding: 8px 14px; border-radius: 4px; cursor: pointer; font-size: 13px; }
button:hover { background: #2d6cc4; }
.range-group { display: flex; align-items: center; gap: 4px; font-size: 12px; color: #aaa; }
.range-group button { background: #333; padding: 6px 10px; }
.error { background: #5a1d1d; color: #ffd6d6; padding: 10px 12px; border-radius: 4px; margin-bottom: 10px; }
.chart-wrap { background: #262626; border-radius: 6px; padding: 10px; margin-bottom: 12px; }
.meta { color: #9a8; font-size: 12px; margin-bottom: 8px; }
.results { font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 13px; }
.line { display: flex; gap: 10px; padding: 3px 6px; border-bottom: 1px solid #2a2a2a; }
.line .ts { color: #888; flex: 0 0 200px; }
.line .txt { white-space: pre-wrap; word-break: break-all; }
.lvl-error .txt { color: #ff6b6b; }
.lvl-warn .txt { color: #ffd166; }
.lvl-debug .txt { color: #888; }
.lvl-info .txt { color: #69c0ff; }
```

- [ ] **Step 3: 写 `web/static/app.js`**

```js
const $ = (id) => document.getElementById(id);
let chart;

function unixNow() { return Math.floor(Date.now() / 1000); }

function toLocalInput(ts) {
  const d = new Date(ts * 1000);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
}

function toUnix(inputVal) {
  if (!inputVal) return 0;
  return Math.floor(new Date(inputVal).getTime() / 1000);
}

async function api(path) {
  const r = await fetch(path);
  const j = await r.json();
  if (!r.ok) throw new Error(j.error || ("HTTP " + r.status));
  return j;
}

function showError(msg) { const b = $("error-bar"); b.textContent = msg; b.hidden = false; }
function hideError() { $("error-bar").hidden = true; }

async function loadLabels() {
  try {
    const j = await api("/api/labels?start=1&end=" + unixNow());
    const sel = $("label-select");
    (j.data || []).forEach((l) => {
      const o = document.createElement("option");
      o.value = l; o.textContent = l;
      sel.appendChild(o);
    });
  } catch (e) { showError(e.message); }
}

$("label-select").addEventListener("change", async (e) => {
  const name = e.target.value;
  if (!name) return;
  try {
    const j = await api("/api/label/" + encodeURIComponent(name) + "/values?start=1&end=" + unixNow());
    const sel = $("value-select");
    sel.disabled = false;
    sel.innerHTML = "";
    (j.data || []).forEach((v) => {
      const o = document.createElement("option");
      o.value = v; o.textContent = v;
      sel.appendChild(o);
    });
  } catch (err) { showError(err.message); }
});

$("add-label").addEventListener("click", () => {
  const name = $("label-select").value;
  const val = $("value-select").value;
  if (!name || !val) { showError("请先选择标签和标签值"); return; }
  const q = $("query");
  const frag = name + '="' + val + '"';
  if (/\{[^}]*\}/.test(q.value)) {
    q.value = q.value.replace(/\{([^}]*)\}/, (m, inner) => "{" + (inner ? inner + ", " + frag : frag) + "}");
  } else {
    q.value = "{" + frag + "}";
  }
});

$("keyword").addEventListener("change", () => {
  const kw = $("keyword").value.trim();
  if (!kw) return;
  $("query").value = $("query").value + ' |= "' + kw + '"';
});

document.querySelectorAll(".range-group button").forEach((b) => {
  b.addEventListener("click", () => {
    const mins = parseInt(b.dataset.mins, 10);
    const end = unixNow();
    $("start").value = toLocalInput(end - mins * 60);
    $("end").value = toLocalInput(end);
  });
});

$("search").addEventListener("click", search);

async function search() {
  hideError();
  const query = $("query").value.trim();
  if (!query) { showError("请输入 LogQL"); return; }
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  if (!start || !end) { showError("请选择时间范围(可点快捷按钮)"); return; }
  const limit = $("limit").value || 1000;
  const direction = $("direction").value;
  const url = "/api/query_range?query=" + encodeURIComponent(query) +
    "&start=" + start + "&end=" + end + "&limit=" + limit + "&direction=" + direction;
  try {
    const j = await api(url);
    renderResults(j);
    renderChart(j.lines || []);
  } catch (e) {
    showError(e.message);
  }
}

function renderResults(res) {
  const box = $("results");
  const lines = res.lines || [];
  box.innerHTML = "";
  const meta = document.createElement("div");
  meta.className = "meta";
  meta.textContent = "返回 " + lines.length + " 行(总计 " + res.total + ")";
  box.appendChild(meta);
  lines.forEach((l) => {
    const row = document.createElement("div");
    row.className = "line lvl-" + (l.level || "info");
    const ts = document.createElement("span");
    ts.className = "ts";
    ts.textContent = l.ts;
    const txt = document.createElement("span");
    txt.className = "txt";
    txt.textContent = l.line;
    row.appendChild(ts);
    row.appendChild(txt);
    box.appendChild(row);
  });
}

function renderChart(lines) {
  const levels = ["error", "warn", "info", "debug"];
  const colors = { error: "#ff6b6b", warn: "#ffd166", info: "#69c0ff", debug: "#888" };
  const buckets = {};
  lines.forEach((l) => {
    const d = new Date(l.ts);
    d.setSeconds(0, 0);
    const key = d.getTime();
    const lvl = levels.includes(l.level) ? l.level : "info";
    buckets[key] = buckets[key] || { error: 0, warn: 0, info: 0, debug: 0 };
    buckets[key][lvl]++;
  });
  const keys = Object.keys(buckets).map(Number).sort((a, b) => a - b);
  const labels = keys.map((k) => new Date(k).toLocaleTimeString());
  const datasets = levels.map((lvl) => ({
    label: lvl,
    stack: "s",
    backgroundColor: colors[lvl],
    data: keys.map((k) => buckets[k][lvl]),
  }));
  if (chart) chart.destroy();
  chart = new Chart($("chart"), {
    type: "bar",
    data: { labels, datasets },
    options: {
      responsive: true,
      plugins: { legend: { labels: { color: "#ddd" } } },
      scales: {
        x: { stacked: true, ticks: { color: "#999" } },
        y: { stacked: true, beginAtZero: true, ticks: { color: "#999" } },
      },
    },
  });
}

loadLabels();
```

- [ ] **Step 4: 验证静态资源存在且无语法明显错误**

Run:
```bash
ls -la web/templates web/static
go build ./...
```
Expected: 三个文件存在;`go build ./...` 通过(main.go 尚未创建,但此时 `go build ./...` 只编译已有包——cmd/server 暂无文件,内部包可编译)。

- [ ] **Step 5: 提交**

```bash
git add web
git commit -m "feat(web): add query UI template, styling, and vanilla JS client"
```

---

## Task 6: main 接线 + 冒烟构建/测试

**Files:**
- Create: `cmd/server/main.go`

**Interfaces:**
- Consumes: `config.Load`(Task 1)、`loki.NewHTTPClient`(Task 3)、`web.New + Register`(Task 4)、模板目录 `web/templates/*`、静态目录 `web/static`

- [ ] **Step 1: 写 `cmd/server/main.go`**

```go
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
```

- [ ] **Step 2: 构建、vet、全量测试**

Run:
```bash
go mod tidy
go build ./...
go vet ./...
go test ./...
```
Expected:
- `go build ./...` 无输出(成功)
- `go vet ./...` 无输出(无问题)
- `go test ./...` 三个包全部 `ok`:`config`、`logparse`、`loki`、`web`

- [ ] **Step 3: 生成二进制并确认可启动(fail-fast 验证)**

Run:
```bash
make build
./bin/log-search --config /nonexistent.yaml; echo "exit=$?"
```
Expected: 配置不存在 → 打印 `load config failed` 日志,`exit=1`(验证 fail-fast,不进入半启动)。

- [ ] **Step 4: 用 example 配置启动(占位 URL,验证路由加载)**

Run:
```bash
cp config.example.yaml config.yaml
./bin/log-search &
sleep 1
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/
curl -s "http://localhost:8080/api/query_range?query=x&start=1&end=99999"
kill %1
```
Expected:
- `GET /` 返回 `200`(模板渲染成功)
- `/api/query_range` 返回 JSON 错误体(因 Loki 指向占位 `localhost:3100` 不可达 → 502,`{"error":"Loki: ..."}`),证明 handler→client 链路接通

- [ ] **Step 5: 提交**

```bash
git add cmd go.mod go.sum config.yaml
git commit -m "feat: wire main server entry, templates, and static assets"
```
(注:`config.yaml` 含用户实际 Loki 地址,若不希望入库可加入 `.gitignore`;此处先提交 example 风格的占位配置。)

---

## Self-Review(写完后的自查结果)

**1. Spec 覆盖**
- 标签/标签值选择 → Task 3 (LabelValues) + Task 5 (UI)
- 时间范围 + 快捷按钮 → Task 4 (parseRange) + Task 5 (range-group)
- 关键词(追加 `|=`) → Task 5 (keyword handler)
- 原生 LogQL 输入框 → Task 5 (#query)
- 查询方向 / 条数限制 → Task 4 (direction/limit) + Task 5 (UI)
- 结果列表 + 级别着色 → Task 2 (DetectLevel) + Task 4 (level fill) + Task 5 (renderResults)
- 直方图统计 → Task 5 (renderChart,基于返回样本)
- YAML 配置 + Loki URL → Task 1
- 错误处理(502/400/fail-fast) → Task 4 + Task 6
- 测试策略(httptest / 表驱动 / mock 注入) → Task 2/3/4

**2. 占位扫描**:无 TBD/TODO/"add error handling";每个代码步骤均给出完整代码。

**3. 类型一致性**:`loki.Client` 接口三方法在 Task 3 定义,Task 4 handler 与 Task 4 测试 mock 签名一致;`Line.Level` 为 `string`,`logparse.Level` 在 handler 中 `string(...)` 转换一致;`Direction` 常量 `Forward/Backward` 在 Task 3/4 一致;`config.LokiCfg.Duration()` 在 Task 1 定义、Task 6 调用一致。
