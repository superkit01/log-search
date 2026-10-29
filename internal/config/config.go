package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"log-search/internal/elasticsearch"
)

type Config struct {
	Server  ServerCfg  `yaml:"server"`
	Storage StorageCfg `yaml:"storage"`
	UI      UICfg      `yaml:"ui"`
}

// StorageCfg 存储后端配置
type StorageCfg struct {
	Type string  `yaml:"type"` // loki, es
	Loki LokiCfg `yaml:"loki"`
	ES   ESCfg   `yaml:"elasticsearch"`
}

type LokiCfg struct {
	URL          string `yaml:"url"`
	Timeout      string `yaml:"timeout"`
	DefaultLimit int    `yaml:"default_limit"`
}

// ESCfg Elasticsearch 配置
type ESCfg struct {
	Fields        elasticsearch.FieldMapping `yaml:"fields"`
	IndexTimezone string                     `yaml:"index_timezone"`
	URL           string                     `yaml:"url"`
	Timeout       string                     `yaml:"timeout"`
	DefaultLimit  int                        `yaml:"default_limit"`
	Index         string                     `yaml:"index"` // 索引名称模式，支持时间戳如 "logs-{yyyy.MM.dd}"
}

type ServerCfg struct {
	Addr string `yaml:"addr"`
}

const (
	DefaultQueryLimit = 1000
	MaxQueryLimit     = 5000
)

type UICfg struct {
	Namespaces []UIOption `yaml:"namespaces"`
	Envs       []UIOption `yaml:"envs"`
	Apps       []string   `yaml:"apps"`
}

type UIOption struct {
	Envs        []UIOption `yaml:"envs"`
	Apps        []string   `yaml:"apps"`
	Description string     `yaml:"description"`
	Label       string     `yaml:"label"`
	Value       string     `yaml:"value"`
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

// Duration 解析 timeout 字符串为 time.Duration。
func (e ESCfg) Duration() (time.Duration, error) {
	return time.ParseDuration(e.Timeout)
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
	// Set default storage type
	if c.Storage.Type == "" {
		c.Storage.Type = "loki"
	}
	// Validate based on storage type
	switch c.Storage.Type {
	case "loki":
		return c.validateLoki()
	case "es":
		return c.validateES()
	default:
		return fmt.Errorf("storage.type must be 'loki' or 'es', got %q", c.Storage.Type)
	}
}

func (c *Config) validateLoki() error {
	if c.Storage.Loki.URL == "" {
		return fmt.Errorf("storage.loki.url is required when type=loki")
	}
	u, err := url.Parse(c.Storage.Loki.URL)
	if err != nil {
		return fmt.Errorf("storage.loki.url invalid %q: must be http(s)://host", c.Storage.Loki.URL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("storage.loki.url invalid %q: must be http(s)://host", c.Storage.Loki.URL)
	}
	if u.Host == "" {
		return fmt.Errorf("storage.loki.url invalid %q: must be http(s)://host", c.Storage.Loki.URL)
	}
	if c.Storage.Loki.Timeout == "" {
		c.Storage.Loki.Timeout = "30s"
	}
	if _, err := time.ParseDuration(c.Storage.Loki.Timeout); err != nil {
		return fmt.Errorf("storage.loki.timeout invalid %q: %w", c.Storage.Loki.Timeout, err)
	}
	if c.Storage.Loki.DefaultLimit <= 0 {
		c.Storage.Loki.DefaultLimit = DefaultQueryLimit
	}
	if c.Storage.Loki.DefaultLimit > MaxQueryLimit {
		return fmt.Errorf("storage.loki.default_limit must be between 1 and %d", MaxQueryLimit)
	}
	return c.validateCommon()
}

func (c *Config) validateES() error {
	if c.Storage.ES.URL == "" {
		return fmt.Errorf("storage.elasticsearch.url is required when type=es")
	}
	u, err := url.Parse(c.Storage.ES.URL)
	if err != nil {
		return fmt.Errorf("storage.elasticsearch.url invalid %q: must be http(s)://host", c.Storage.ES.URL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("storage.elasticsearch.url invalid %q: must be http(s)://host", c.Storage.ES.URL)
	}
	if u.Host == "" {
		return fmt.Errorf("storage.elasticsearch.url invalid %q: must be http(s)://host", c.Storage.ES.URL)
	}
	if c.Storage.ES.Timeout == "" {
		c.Storage.ES.Timeout = "30s"
	}
	if _, err := time.ParseDuration(c.Storage.ES.Timeout); err != nil {
		return fmt.Errorf("storage.elasticsearch.timeout invalid %q: %w", c.Storage.ES.Timeout, err)
	}
	if c.Storage.ES.DefaultLimit <= 0 {
		c.Storage.ES.DefaultLimit = DefaultQueryLimit
	}
	if c.Storage.ES.DefaultLimit > MaxQueryLimit {
		return fmt.Errorf("storage.elasticsearch.default_limit must be between 1 and %d", MaxQueryLimit)
	}
	if c.Storage.ES.Index == "" {
		return fmt.Errorf("storage.elasticsearch.index is required when type=es")
	}
	if c.Storage.ES.IndexTimezone == "" {
		c.Storage.ES.IndexTimezone = "UTC"
	}
	if _, err := time.LoadLocation(c.Storage.ES.IndexTimezone); err != nil {
		return fmt.Errorf("storage.elasticsearch.index_timezone: %w", err)
	}
	return c.validateCommon()
}

func (c *Config) validateCommon() error {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	return validateUIOptions(&c.UI)
}

// validateUIOptions 验证并规范化 UI 配置，trim 空白、应用默认值。
func validateUIOptions(ui *UICfg) error {
	seen := map[string]bool{}
	for i := range ui.Namespaces {
		option := &ui.Namespaces[i]
		option.Value = strings.TrimSpace(option.Value)
		option.Label = strings.TrimSpace(option.Label)
		option.Description = strings.TrimSpace(option.Description)
		option.Apps = compactStrings(option.Apps)
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`).MatchString(option.Value) {
			return fmt.Errorf("ui.namespaces[%d].value must be a literal namespace", i)
		}
		if seen[option.Value] {
			return fmt.Errorf("duplicate ui.namespaces value %q", option.Value)
		}
		seen[option.Value] = true
		if option.Label == "" {
			option.Label = option.Value
		}
	}

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
	for i := range ui.Namespaces {
		option := &ui.Namespaces[i]
		if len(option.Envs) == 0 {
			option.Envs = append([]UIOption(nil), ui.Envs...)
		}
		scoped := UICfg{Envs: option.Envs}
		if err := validateUIOptions(&scoped); err != nil {
			return fmt.Errorf("ui.namespaces[%d]: %w", i, err)
		}
		option.Envs = scoped.Envs
	}
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
