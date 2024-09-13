// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/miekg/dns"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dnsmasq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Dnsmasq {
	return &Dnsmasq{
		Config: Config{
			Protocol: "udp",
			Address:  "127.0.0.1:53",
			Timeout:  confopt.Duration(time.Second),
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
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Protocol    string           `yaml:"protocol,omitempty" json:"protocol"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	Dnsmasq struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		dnsClient    dnsClient
		newDNSClient func(network string, timeout time.Duration) dnsClient
	}
	dnsClient interface {
		Exchange(msg *dns.Msg, address string) (resp *dns.Msg, rtt time.Duration, err error)
	}
)

func (d *Dnsmasq) Configuration() any {
	return d.Config
}

func (d *Dnsmasq) Init() error {
	err := d.validateConfig()
	if err != nil {
		d.Errorf("config validation: %v", err)
		return err
	}

	client, err := d.initDNSClient()
	if err != nil {
		d.Errorf("init DNS client: %v", err)
		return err
	}
	d.dnsClient = client

	charts, err := d.initCharts()
	if err != nil {
		d.Errorf("init charts: %v", err)
		return err
	}
	d.charts = charts

	return nil
}

func (d *Dnsmasq) Check() error {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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

func (d *Dnsmasq) Cleanup() {}
