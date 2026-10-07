// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type Config struct {
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Driver      string           `yaml:"driver" json:"driver"`
	Device      int32            `yaml:"device" json:"device"`
	Hostname    string           `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Port        int              `yaml:"port,omitempty" json:"port,omitempty"`
	Username    string           `yaml:"username,omitempty" json:"username,omitempty"`
	Password    string           `yaml:"password,omitempty" json:"password,omitempty"`
	CollectSEL  bool             `yaml:"collect_sel" json:"collect_sel"`
}

func defaultConfig() Config {
	return Config{UpdateEvery: defaultUpdateEvery, Timeout: confopt.Duration(5 * time.Second),
		Driver: "open", Port: 623, CollectSEL: true}
}
func (c Config) validate() error {
	if c.UpdateEvery < defaultUpdateEvery {
		return fmt.Errorf("update_every must be at least %d seconds", defaultUpdateEvery)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	switch c.Driver {
	case "open":
		if c.Device < 0 {
			return fmt.Errorf("device must not be negative")
		}
		if c.Hostname != "" || c.Username != "" || c.Password != "" || c.Port != 623 {
			return fmt.Errorf("hostname, credentials and a custom port require driver lan or lanplus")
		}
		if c.Vnode != "" {
			return fmt.Errorf("vnode is only supported for remote IPMI")
		}
	case "lan", "lanplus":
		if strings.TrimSpace(c.Hostname) == "" {
			return fmt.Errorf("hostname is required for remote IPMI")
		}
		if c.Port < 1 || c.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		if c.Device != 0 {
			return fmt.Errorf("device is only supported with driver open")
		}
	default:
		return fmt.Errorf("driver must be open, lan or lanplus")
	}
	return nil
}
