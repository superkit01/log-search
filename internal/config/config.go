package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerCfg `yaml:"server"`
	Loki   LokiCfg   `yaml:"loki"`
	UI     UICfg     `yaml:"ui"`
}

type ServerCfg struct {
	Addr string `yaml:"addr"`
}

type LokiCfg struct {
	URL          string `yaml:"url"`
	Timeout      string `yaml:"timeout"`
	DefaultLimit int    `yaml:"default_limit"`
}

const (
	DefaultQueryLimit = 1000
	MaxQueryLimit     = 5000
)

type UICfg struct {
	Envs []UIOption `yaml:"envs"`
	Apps []string   `yaml:"apps"`
}

type UIOption struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
}

func DefaultUICfg() UICfg {
	return UICfg{Envs: []UIOption{
		{Label: "演示", Value: "demo"},
	}}
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
	// 在 YAML 解析前做 ${VAR} / ${VAR:-default} 文本级展开(便于容器化)。
	b = []byte(expandEnv(string(b)))
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// envRe 匹配 ${VAR} 与 ${VAR:-default}。
var envRe = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// expandEnv 展开字符串中的 ${VAR} 与 ${VAR:-default}(bash/docker-compose 风格)。
//
//	${VAR}      → VAR 的值(未设/为空 → 空串)
//	${VAR:-x}   → VAR 已设且非空时取其值,否则取 x
//
// 不含 "${" 的串直接返回(快速路径)。
func expandEnv(s string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	return envRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := envRe.FindStringSubmatch(m)
		if v := os.Getenv(parts[1]); v != "" {
			return v
		}
		return parts[2]
	})
}

func (c *Config) validate() error {
	if c.Loki.URL == "" {
		return fmt.Errorf("loki.url is required")
	}
	// Validate URL scheme and host
	u, err := url.Parse(c.Loki.URL)
	if err != nil {
		return fmt.Errorf("loki.url invalid %q: must be http(s)://host", c.Loki.URL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("loki.url invalid %q: must be http(s)://host", c.Loki.URL)
	}
	if u.Host == "" {
		return fmt.Errorf("loki.url invalid %q: must be http(s)://host", c.Loki.URL)
	}
	if c.Loki.Timeout == "" {
		c.Loki.Timeout = "30s"
	}
	if _, err := time.ParseDuration(c.Loki.Timeout); err != nil {
		return fmt.Errorf("loki.timeout invalid %q: %w", c.Loki.Timeout, err)
	}
	if c.Loki.DefaultLimit <= 0 {
		c.Loki.DefaultLimit = DefaultQueryLimit
	}
	if c.Loki.DefaultLimit > MaxQueryLimit {
		return fmt.Errorf("loki.default_limit must be between 1 and %d", MaxQueryLimit)
	}
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if err := validateUIOptions(&c.UI); err != nil {
		return err
	}
	return nil
}

// validateUIOptions 验证并规范化 UI 配置，trim 空白、应用默认值。
func validateUIOptions(ui *UICfg) error {
	if len(ui.Envs) == 0 {
		ui.Envs = DefaultUICfg().Envs
	}
	for i := range ui.Envs {
		ui.Envs[i].Label = strings.TrimSpace(ui.Envs[i].Label)
		ui.Envs[i].Value = strings.TrimSpace(ui.Envs[i].Value)
		if ui.Envs[i].Value == "" {
			return fmt.Errorf("ui.envs[%d].value is required", i)
		}
		if ui.Envs[i].Label == "" {
			ui.Envs[i].Label = ui.Envs[i].Value
		}
	}
	ui.Apps = compactStrings(ui.Apps)
	return nil
}

func compactStrings(in []string) []string {
	out := in[:0]
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
