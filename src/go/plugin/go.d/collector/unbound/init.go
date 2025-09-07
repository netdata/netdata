// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"crypto/tls"
	"errors"
	"net"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/unbound/config"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

func (c *Collector) initConfig() (enabled bool) {
	if c.ConfPath == "" {
		c.Info("'conf_path' not set, skipping parameters auto detection")
		return true
	}

	c.Infof("reading '%s'", c.ConfPath)
	cfg, err := config.Parse(c.ConfPath)
	if err != nil {
		c.Warningf("%v, skipping parameters auto detection", err)
		return true
	}

	if cfg.Empty() {
		c.Debug("empty configuration")
		return true
	}

	if enabled, ok := cfg.ControlEnabled(); ok && !enabled {
		c.Info("remote control is disabled in the configuration file")
		return false
	}

	c.applyConfig(cfg)
	return true
}

func (c *Collector) applyConfig(cfg *config.UnboundConfig) {
	c.Infof("applying configuration: %s", cfg)
	if cumulative, ok := cfg.Cumulative(); ok && cumulative != c.Cumulative.Bool() {
		c.Debugf("changing 'cumulative_stats': %v => %v", c.Cumulative, cumulative)
		c.Cumulative = confopt.FlexBool(cumulative)
	}
	if useCert, ok := cfg.ControlUseCert(); ok && useCert != c.UseTLS.Bool() {
		c.Debugf("changing 'use_tls': %v => %v", c.UseTLS, useCert)
		c.UseTLS = confopt.FlexBool(useCert)
	}
	if keyFile, ok := cfg.ControlKeyFile(); ok && keyFile != c.TLSKey {
		c.Debugf("changing 'tls_key': '%s' => '%s'", c.TLSKey, keyFile)
		c.TLSKey = keyFile
	}
	if certFile, ok := cfg.ControlCertFile(); ok && certFile != c.TLSCert {
		c.Debugf("changing 'tls_cert': '%s' => '%s'", c.TLSCert, certFile)
		c.TLSCert = certFile
	}
	if iface, ok := cfg.ControlInterface(); ok && adjustControlInterface(iface) != c.Address {
		address := adjustControlInterface(iface)
		c.Debugf("changing 'address': '%s' => '%s'", c.Address, address)
		c.Address = address
	}
	if port, ok := cfg.ControlPort(); ok && !socket.IsUnixSocket(c.Address) {
		if host, curPort, err := net.SplitHostPort(c.Address); err == nil && curPort != port {
			address := net.JoinHostPort(host, port)
			c.Debugf("changing 'address': '%s' => '%s'", c.Address, address)
			c.Address = address
		}
	}
}

func (c *Collector) initClient() (err error) {
	var tlsCfg *tls.Config
	useTLS := !socket.IsUnixSocket(c.Address) && c.UseTLS.Bool()

	if useTLS && (c.TLSConfig.TLSCert == "" || c.TLSConfig.TLSKey == "") {
		return errors.New("'tls_cert' or 'tls_key' is missing")
	}

	if useTLS {
		if tlsCfg, err = tlscfg.NewTLSConfig(c.TLSConfig); err != nil {
			return err
		}
	}

	c.client = socket.New(socket.Config{
		Address: c.Address,
		Timeout: c.Timeout.Duration(),
		TLSConf: tlsCfg,
	})
	return nil
}

func adjustControlInterface(value string) string {
	if socket.IsUnixSocket(value) {
		return value
	}
	if value == "0.0.0.0" {
		value = "127.0.0.1"
	}
	return net.JoinHostPort(value, "8953")
}
