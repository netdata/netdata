// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	defaultInterval = time.Minute
	defaultTimeout  = 2 * time.Second

	responseBodyLimit = 10 * 1024 * 1024

	formatAuto = "auto"
	formatJSON = "json"
	formatYAML = "yaml"
)

type Config struct {
	Source string `yaml:"-" json:"-"`

	web.HTTPConfig `yaml:",inline" json:""`

	Interval *confopt.LongDuration `yaml:"interval,omitempty" json:"interval,omitempty"`
	Format   string                `yaml:"format,omitempty" json:"format,omitempty"`
}

func (c Config) validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("url is required")
	}

	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url host is required")
	}

	switch c.format() {
	case formatAuto, formatJSON, formatYAML:
	default:
		return fmt.Errorf("unsupported format %q", c.Format)
	}

	if c.Interval != nil && c.Interval.Duration() < 0 {
		return errors.New("interval cannot be negative")
	}

	return nil
}

func (c Config) interval() time.Duration {
	if c.Interval == nil {
		return defaultInterval
	}
	return c.Interval.Duration()
}

func (c Config) clientConfig() web.ClientConfig {
	cfg := c.ClientConfig
	if cfg.Timeout.Duration() <= 0 {
		cfg.Timeout = confopt.Duration(defaultTimeout)
	}
	return cfg
}

func (c Config) format() string {
	if v := strings.TrimSpace(strings.ToLower(c.Format)); v != "" {
		return v
	}
	return formatAuto
}
