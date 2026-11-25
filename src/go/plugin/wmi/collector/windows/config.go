// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"errors"
	"net/netip"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	defaultUpdateEvery = 30
	defaultTimeout     = 15 * time.Second
)

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty"               json:"vnode,omitempty"`
	UpdateEvery        int              `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Host               string           `yaml:"host"                          json:"host"`
	Auth               string           `yaml:"auth"                          json:"auth"`
	Username           string           `yaml:"username"                      json:"username"`
	Password           string           `yaml:"password"                      json:"password"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty"             json:"timeout,omitempty"`
	Collectors         Collectors       `yaml:"collectors,omitempty"          json:"collectors,omitzero"`
}

// A missing option enables that group. A pointer keeps an explicit false
// distinct from omission when the framework decodes or reserializes a job.
type Collectors struct {
	CPU     *bool `yaml:"cpu,omitempty"     json:"cpu,omitempty"`
	Memory  *bool `yaml:"memory,omitempty"  json:"memory,omitempty"`
	Network *bool `yaml:"network,omitempty" json:"network,omitempty"`
	Disk    *bool `yaml:"disk,omitempty"    json:"disk,omitempty"`
}

func defaultConfig() Config {
	return Config{
		UpdateEvery: defaultUpdateEvery,
		Timeout:     confopt.Duration(defaultTimeout),
	}
}

func (c *Config) validate() error {
	c.Host = strings.TrimSpace(c.Host)
	if !validHost(c.Host) {
		return errors.New("host must be a Windows hostname or IP address without a URL or port")
	}
	if c.Auth != "ntlm" {
		return errors.New("auth must be ntlm")
	}
	if strings.TrimSpace(c.Username) == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if !enabled(c.Collectors.CPU) && !enabled(c.Collectors.Memory) &&
		!enabled(c.Collectors.Network) && !enabled(c.Collectors.Disk) {
		return errors.New("at least one collector group must be enabled")
	}
	return nil
}

func validHost(host string) bool {
	if host == "" || host == "." || strings.ContainsAny(host, " /\\?#@[]") ||
		strings.IndexFunc(host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	return !strings.ContainsAny(host, ":%")
}

func enabled(option *bool) bool { return option == nil || *option }
