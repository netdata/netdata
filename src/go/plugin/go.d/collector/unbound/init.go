// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"crypto/tls"
	"errors"
	"net"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/unbound/config"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

func (u *Unbound) initConfig() (enabled bool) {
	if u.ConfPath == "" {
		u.Info("'conf_path' not set, skipping parameters auto detection")
		return true
	}

	u.Infof("reading '%s'", u.ConfPath)
	cfg, err := config.Parse(u.ConfPath)
	if err != nil {
		u.Warningf("%v, skipping parameters auto detection", err)
		return true
	}

	if cfg.Empty() {
		u.Debug("empty configuration")
		return true
	}

	if enabled, ok := cfg.ControlEnabled(); ok && !enabled {
		u.Info("remote control is disabled in the configuration file")
		return false
	}

	u.applyConfig(cfg)
	return true
}

func (u *Unbound) applyConfig(cfg *config.UnboundConfig) {
	u.Infof("applying configuration: %s", cfg)
	if cumulative, ok := cfg.Cumulative(); ok && cumulative != u.Cumulative {
		u.Debugf("changing 'cumulative_stats': %v => %v", u.Cumulative, cumulative)
		u.Cumulative = cumulative
	}
	if useCert, ok := cfg.ControlUseCert(); ok && useCert != u.UseTLS {
		u.Debugf("changing 'use_tls': %v => %v", u.UseTLS, useCert)
		u.UseTLS = useCert
	}
	if keyFile, ok := cfg.ControlKeyFile(); ok && keyFile != u.TLSKey {
		u.Debugf("changing 'tls_key': '%s' => '%s'", u.TLSKey, keyFile)
		u.TLSKey = keyFile
	}
	if certFile, ok := cfg.ControlCertFile(); ok && certFile != u.TLSCert {
		u.Debugf("changing 'tls_cert': '%s' => '%s'", u.TLSCert, certFile)
		u.TLSCert = certFile
	}
	if iface, ok := cfg.ControlInterface(); ok && adjustControlInterface(iface) != u.Address {
		address := adjustControlInterface(iface)
		u.Debugf("changing 'address': '%s' => '%s'", u.Address, address)
		u.Address = address
	}
	if port, ok := cfg.ControlPort(); ok && !socket.IsUnixSocket(u.Address) {
		if host, curPort, err := net.SplitHostPort(u.Address); err == nil && curPort != port {
			address := net.JoinHostPort(host, port)
			u.Debugf("changing 'address': '%s' => '%s'", u.Address, address)
			u.Address = address
		}
	}
}

func (u *Unbound) initClient() (err error) {
	var tlsCfg *tls.Config
	useTLS := !socket.IsUnixSocket(u.Address) && u.UseTLS

	if useTLS && (u.TLSConfig.TLSCert == "" || u.TLSConfig.TLSKey == "") {
		return errors.New("'tls_cert' or 'tls_key' is missing")
	}

	if useTLS {
		if tlsCfg, err = tlscfg.NewTLSConfig(u.TLSConfig); err != nil {
			return err
		}
	}

	u.client = socket.New(socket.Config{
		Address: u.Address,
		Timeout: u.Timeout.Duration(),
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
