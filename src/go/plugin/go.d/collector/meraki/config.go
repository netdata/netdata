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
	Vnode                         string           `yaml:"vnode,omitempty"                            json:"vnode"`
	UpdateEvery                   int              `yaml:"update_every,omitempty"                     json:"update_every"`
	AutoDetectionRetry            int              `yaml:"autodetection_retry,omitempty"              json:"autodetection_retry"`
	URL                           string           `yaml:"url"                                        json:"url"`
	OrganizationID                string           `yaml:"organization_id"                            json:"organization_id"`
	APIKey                        string           `yaml:"api_key,omitempty"                          json:"api_key"`
	BearerTokenFile               string           `yaml:"bearer_token_file,omitempty"                json:"bearer_token_file"`
	Timeout                       confopt.Duration `yaml:"timeout,omitempty"                          json:"timeout"`
	ProxyURL                      string           `yaml:"proxy_url,omitempty"                        json:"proxy_url"`
	CollectSwitchPortTraffic      bool             `yaml:"collect_switch_port_traffic,omitempty"      json:"collect_switch_port_traffic"`
	CollectSwitchPortPackets      bool             `yaml:"collect_switch_port_packets,omitempty"      json:"collect_switch_port_packets"`
	CollectApplianceUplinkTraffic bool             `yaml:"collect_appliance_uplink_traffic,omitempty" json:"collect_appliance_uplink_traffic"`
	CollectApplianceUplinkHealth  bool             `yaml:"collect_appliance_uplink_health,omitempty"  json:"collect_appliance_uplink_health"`
	tlscfg.TLSConfig              `yaml:",inline" json:""`
}

// validateConfig rejects invalid options at Init, where the framework does not
// retry them. URL and credential rules are enforced by merakiapi.New.
func (c *Collector) validateConfig() error {
	id := c.OrganizationID
	if id == "" || strings.TrimSpace(id) != id || id == "." || id == ".." {
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
