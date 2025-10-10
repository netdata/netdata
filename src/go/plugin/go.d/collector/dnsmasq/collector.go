// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/miekg/dns"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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

func New() *Collector {
	return &Collector{
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
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address" json:"address"`
	Protocol           string           `yaml:"protocol,omitempty" json:"protocol"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	client, err := c.initDNSClient()
	if err != nil {
		return fmt.Errorf("init DNS client: %v", err)
	}
	c.dnsClient = client

	charts, err := c.initCharts()
	if err != nil {
		return fmt.Errorf("init charts: %v", err)
	}
	c.charts = charts

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	ms, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (c *Collector) Cleanup(context.Context) {}
