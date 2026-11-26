// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	defaultUpdateEvery = 30
	defaultTimeout     = 15 * time.Second

	transportDCOM  = "dcom"
	transportWinRM = "winrm"
	authNTLM       = "ntlm"
)

type Config struct {
	Vnode              string                      `yaml:"vnode,omitempty"               json:"vnode,omitempty"`
	UpdateEvery        int                         `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	AutoDetectionRetry int                         `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Host               string                      `yaml:"host"                          json:"host"`
	Transport          confopt.Enum[transportSpec] `yaml:"transport,omitempty"           json:"transport,omitempty"`
	TLSCA              string                      `yaml:"tls_ca,omitempty"              json:"tls_ca,omitempty"`
	Auth               string                      `yaml:"auth"                          json:"auth"`
	Username           string                      `yaml:"username"                      json:"username"`
	Password           string                      `yaml:"password"                      json:"password"`
	Timeout            confopt.Duration            `yaml:"timeout,omitempty"             json:"timeout,omitempty"`
	Collectors         Collectors                  `yaml:"collectors,omitempty"          json:"collectors,omitzero"`
}

type transportSpec struct{}

func (transportSpec) Values() []string { return []string{transportDCOM, transportWinRM} }
func (transportSpec) Default() string  { return transportDCOM }

// Collectors switches the metric groups. An omitted group is enabled. Pointers
// keep an explicit false distinct from omission when the framework decodes or
// reserializes a job.
type Collectors struct {
	CPU     *bool `yaml:"cpu,omitempty"     json:"cpu,omitempty"`
	Memory  *bool `yaml:"memory,omitempty"  json:"memory,omitempty"`
	Network *bool `yaml:"network,omitempty" json:"network,omitempty"`
	Disk    *bool `yaml:"disk,omitempty"    json:"disk,omitempty"`
}

func groupEnabled(option *bool) bool { return option == nil || *option }

func defaultConfig() Config {
	return Config{
		UpdateEvery: defaultUpdateEvery,
		Transport:   transportDCOM,
		Timeout:     confopt.Duration(defaultTimeout),
	}
}

// validate normalizes host and transport and reports the first invalid option.
// The query transports rely on it and do not validate their inputs again.
func (c *Config) validate() error {
	c.Host = strings.TrimSpace(c.Host)
	if !validHost(c.Host) {
		return errors.New("host must be a Windows hostname or IP address without a URL or port")
	}
	if err := c.Transport.Validate(); err != nil {
		return fmt.Errorf("transport %w", err)
	}
	c.Transport = c.Transport.Normalized()
	if c.TLSCA != "" && c.Transport != transportWinRM {
		return errors.New("tls_ca requires the winrm transport")
	}
	if c.Auth != authNTLM {
		return errors.New("auth must be ntlm")
	}
	if logonAccount(c.Username) == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	groups := c.Collectors
	if !groupEnabled(groups.CPU) && !groupEnabled(groups.Memory) && !groupEnabled(groups.Network) &&
		!groupEnabled(groups.Disk) {
		return errors.New("at least one collector group must be enabled")
	}
	return nil
}

// validHost accepts hostnames and IP literals, including scoped IPv6 addresses.
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

// logonAccount returns the account part of a DOMAIN\user, user@domain or plain logon name.
func logonAccount(username string) string {
	if _, account, ok := strings.Cut(username, `\`); ok {
		return strings.TrimSpace(account)
	}
	account, _, _ := strings.Cut(username, "@")
	return strings.TrimSpace(account)
}
