// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/miekg/dns"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dnsmasq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Dnsmasq {
	return &Dnsmasq{
		Config: Config{
			Protocol: "udp",
			Address:  "127.0.0.1:53",
			Timeout:  web.Duration{Duration: time.Second},
		},

		newDNSClient: func(network string, timeout time.Duration) dnsClient {
			return &dns.Client{
				Net:     network,
				Timeout: timeout,
			}
		},
	}
}

type Config struct {
	Protocol string       `yaml:"protocol"`
	Address  string       `yaml:"address"`
	Timeout  web.Duration `yaml:"timeout"`
}

type (
	Dnsmasq struct {
		module.Base
		Config `yaml:",inline"`

		newDNSClient func(network string, timeout time.Duration) dnsClient
		dnsClient    dnsClient

		charts *module.Charts
	}

	dnsClient interface {
		Exchange(msg *dns.Msg, address string) (resp *dns.Msg, rtt time.Duration, err error)
	}
)

func (d *Dnsmasq) Init() bool {
	err := d.validateConfig()
	if err != nil {
		d.Errorf("config validation: %v", err)
		return false
	}

	client, err := d.initDNSClient()
	if err != nil {
		d.Errorf("init DNS client: %v", err)
		return false
	}
	d.dnsClient = client

	charts, err := d.initCharts()
	if err != nil {
		d.Errorf("init charts: %v", err)
		return false
	}
	d.charts = charts

	return true
}

func (d *Dnsmasq) Check() bool {
	return len(d.Collect()) > 0
}

func (d *Dnsmasq) Charts() *module.Charts {
	return d.charts
}

func (d *Dnsmasq) Collect() map[string]int64 {
	ms, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (Dnsmasq) Cleanup() {}
