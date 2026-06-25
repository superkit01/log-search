package config

import (
	"fmt"
	"net/url"
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
		c.Loki.DefaultLimit = 1000
	}
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	return nil
}
