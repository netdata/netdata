// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty"               json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty"        json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Source             string           `yaml:"source"                        json:"source"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty"             json:"timeout"`
	CheckFullChain     bool             `yaml:"check_full_chain"              json:"check_full_chain"`
	CheckRevocation    bool             `yaml:"check_revocation_status"       json:"check_revocation_status"`
	TLSServerName      string           `yaml:"tls_server_name,omitempty"     json:"tls_server_name"`
	tlscfg.TLSConfig   `yaml:",inline" json:""`
}

func (c *Collector) validateConfig() error {
	if c.Source == "" {
		return errors.New("source is not set")
	}
	if c.Timeout.Duration() <= 0 {
		return errors.New("timeout must be greater than zero")
	}
	return nil
}
