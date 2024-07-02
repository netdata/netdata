// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"strings"
)

// UnboundConfig represents Unbound configuration file.
type UnboundConfig struct {
	cumulative string // statistics-cumulative
	enable     string // control-enable
	iface      string // control-interface
	port       string // control-port
	useCert    string // control-use-cert
	keyFile    string // control-key-file
	certFile   string // control-cert-file
}

func (c UnboundConfig) String() string {
	format := strings.Join([]string{
		"[",
		`"statistics-cumulative": '%s', `,
		`"control-enable": '%s', `,
		`"control-interface": '%s', `,
		`"control-port": '%s', `,
		`"control-user-cert": '%s', `,
		`"control-key-file": '%s', `,
		`"control-cert-file": '%s'`,
		"]",
	}, "")
	return fmt.Sprintf(format, c.cumulative, c.enable, c.iface, c.port, c.useCert, c.keyFile, c.certFile)
}

func (c UnboundConfig) Empty() bool                      { return c == UnboundConfig{} }
func (c UnboundConfig) Cumulative() (bool, bool)         { return c.cumulative == "yes", c.cumulative != "" }
func (c UnboundConfig) ControlEnabled() (bool, bool)     { return c.enable == "yes", c.enable != "" }
func (c UnboundConfig) ControlInterface() (string, bool) { return c.iface, c.iface != "" }
func (c UnboundConfig) ControlPort() (string, bool)      { return c.port, c.port != "" }
func (c UnboundConfig) ControlUseCert() (bool, bool)     { return c.useCert == "yes", c.useCert != "" }
func (c UnboundConfig) ControlKeyFile() (string, bool)   { return c.keyFile, c.keyFile != "" }
func (c UnboundConfig) ControlCertFile() (string, bool)  { return c.certFile, c.certFile != "" }

func fromOptions(options []option) *UnboundConfig {
	cfg := &UnboundConfig{}
	for _, opt := range options {
		switch opt.name {
		default:
		case optInterface:
			applyControlInterface(cfg, opt.value)
		case optCumulative:
			cfg.cumulative = opt.value
		case optEnable:
			cfg.enable = opt.value
		case optPort:
			cfg.port = opt.value
		case optUseCert:
			cfg.useCert = opt.value
		case optKeyFile:
			cfg.keyFile = opt.value
		case optCertFile:
			cfg.certFile = opt.value
		}
	}
	return cfg
}

// Unbound doesn't allow to query stats from unix socket when control-interface is enabled on ip interface.
func applyControlInterface(cfg *UnboundConfig, value string) {
	if cfg.iface == "" || !isUnixSocket(value) || isUnixSocket(cfg.iface) {
		cfg.iface = value
	}
}

func isUnixSocket(address string) bool {
	return strings.HasPrefix(address, "/")
}
