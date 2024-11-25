// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/miekg/dns"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dns_query", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *DNSQuery {
	return &DNSQuery{
		Config: Config{
			Timeout:     confopt.Duration(time.Second * 2),
			Network:     "udp",
			RecordTypes: []string{"A"},
			Port:        53,
		},
		newDNSClient: func(network string, timeout time.Duration) dnsClient {
			return &dns.Client{
				Net:         network,
				ReadTimeout: timeout,
			}
		},
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Domains     []string         `yaml:"domains" json:"domains"`
	Servers     []string         `yaml:"servers" json:"servers"`
	Network     string           `yaml:"network,omitempty" json:"network"`
	RecordType  string           `yaml:"record_type,omitempty" json:"record_type"`
	RecordTypes []string         `yaml:"record_types,omitempty" json:"record_types"`
	Port        int              `yaml:"port,omitempty" json:"port"`
}

type (
	DNSQuery struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		dnsClient    dnsClient
		newDNSClient func(network string, duration time.Duration) dnsClient

		recordTypes map[string]uint16
	}
	dnsClient interface {
		Exchange(msg *dns.Msg, address string) (response *dns.Msg, rtt time.Duration, err error)
	}
)

func (d *DNSQuery) Configuration() any {
	return d.Config
}

func (d *DNSQuery) Init() error {
	if err := d.verifyConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	rt, err := d.initRecordTypes()
	if err != nil {
		return fmt.Errorf("init record type: %v", err)
	}
	d.recordTypes = rt

	charts, err := d.initCharts()
	if err != nil {
		return fmt.Errorf("init charts: %v", err)
	}
	d.charts = charts

	return nil
}

func (d *DNSQuery) Check() error {
	return nil
}

func (d *DNSQuery) Charts() *module.Charts {
	return d.charts
}

func (d *DNSQuery) Collect() map[string]int64 {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (d *DNSQuery) Cleanup() {}
