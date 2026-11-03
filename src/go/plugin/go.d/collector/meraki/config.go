// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"errors"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

const (
	// Meraki refreshes device availability every five minutes.
	defaultUpdateEvery = 300
	defaultTimeout     = confopt.Duration(60 * time.Second)
)

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty"               json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty"        json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	URL                string           `yaml:"url"                           json:"url"`
	OrganizationID     string           `yaml:"organization_id"               json:"organization_id"`
	APIKey             string           `yaml:"api_key,omitempty"             json:"api_key"`
	BearerTokenFile    string           `yaml:"bearer_token_file,omitempty"   json:"bearer_token_file"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty"             json:"timeout"`
	ProxyURL           string           `yaml:"proxy_url,omitempty"           json:"proxy_url"`
	tlscfg.TLSConfig   `yaml:",inline" json:""`
}

// validateConfig checks options the client does not validate itself. URL and
// credential rules are enforced by merakiapi.New.
func (c *Collector) validateConfig() error {
	if c.OrganizationID == "" || strings.TrimSpace(c.OrganizationID) != c.OrganizationID {
		return errors.New("organization_id must be a nonempty organization ID without surrounding whitespace")
	}
	if c.Timeout.Duration() <= 0 {
		return errors.New("timeout must be greater than zero")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls_cert and tls_key must be configured together")
	}
	return nil
}
