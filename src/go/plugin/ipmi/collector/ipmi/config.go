// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	defaultUpdateEvery = 5
	minUpdateEvery     = 5
	defaultTimeout     = 5 * time.Second

	driverOpen            = "open"
	driverLAN             = "lan"
	driverLANPlus         = "lanplus"
	defaultPort           = 623
	defaultPrivilegeLevel = "user"
)

type Config struct {
	Vnode          string           `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	Hostname       string           `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Port           int              `yaml:"port,omitempty" json:"port,omitempty"`
	Username       string           `yaml:"username,omitempty" json:"username,omitempty"`
	Password       string           `yaml:"password,omitempty" json:"password,omitempty"`
	PrivilegeLevel string           `yaml:"privilege_level,omitempty" json:"privilege_level,omitempty"`
	UpdateEvery    int              `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	Timeout        confopt.Duration `yaml:"timeout,omitempty"      json:"timeout,omitempty"`
	Driver         string           `yaml:"driver"                 json:"driver"`
	Device         int32            `yaml:"device"                 json:"device"`
	CollectSEL     bool             `yaml:"collect_sel"            json:"collect_sel"`
}

func defaultConfig() Config {
	return Config{
		UpdateEvery: defaultUpdateEvery,
		Timeout:     confopt.Duration(defaultTimeout),
		Driver:      driverOpen,
		CollectSEL:  true,
	}
}

func (c Config) validate() error {
	if c.UpdateEvery < minUpdateEvery {
		return fmt.Errorf("update_every must be at least %d seconds", minUpdateEvery)
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	switch c.Driver {
	case driverOpen:
		if c.Device < 0 {
			return errors.New("device must not be negative")
		}
	case driverLAN, driverLANPlus:
		if c.Hostname == "" || strings.ContainsAny(c.Hostname, " /\\?#@\t\r\n[]") {
			return errors.New("hostname must be a BMC hostname or IP address without a URL or port")
		}
		if _, err := netip.ParseAddr(c.Hostname); err != nil && strings.Contains(c.Hostname, ":") {
			return errors.New("hostname must not include a port")
		}
		if c.Port < 1 || c.Port > 65535 {
			return errors.New("port must be between 1 and 65535")
		}
		if len(c.Username) > 16 {
			return errors.New("username must not exceed 16 bytes")
		}
		if c.Driver == driverLAN && len(c.Password) > 16 {
			return errors.New("LAN password must not exceed 16 bytes")
		}
		switch c.PrivilegeLevel {
		case "user", "operator", "administrator":
		default:
			return errors.New("privilege_level must be user, operator or administrator")
		}
	default:
		return fmt.Errorf("unsupported driver %q: use open, lan or lanplus", c.Driver)
	}
	return nil
}

func (c *Config) normalize() {
	switch c.Driver {
	case driverOpen:
		c.Hostname, c.Username, c.Password, c.PrivilegeLevel = "", "", "", ""
		c.Port = 0
	case driverLAN, driverLANPlus:
		c.Device = 0
		c.Hostname = strings.TrimSpace(c.Hostname)
		if c.Port == 0 {
			c.Port = defaultPort
		}
		if c.PrivilegeLevel == "" {
			c.PrivilegeLevel = defaultPrivilegeLevel
		}
	}
}
