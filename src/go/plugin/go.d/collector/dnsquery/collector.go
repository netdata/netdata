// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/miekg/dns"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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

func New() *Collector {
	return &Collector{
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
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode"`
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
	Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.verifyConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	if err := c.initServers(); err != nil {
		return fmt.Errorf("failed to initialize servers: %v", err)
	}

	rt, err := c.initRecordTypes()
	if err != nil {
		return fmt.Errorf("init record type: %v", err)
	}
	c.recordTypes = rt

	charts, err := c.initCharts()
	if err != nil {
		return fmt.Errorf("init charts: %v", err)
	}
	c.charts = charts

	return nil
}

func (c *Collector) Check(context.Context) error {
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {}
