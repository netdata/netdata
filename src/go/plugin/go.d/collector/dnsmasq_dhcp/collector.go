// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dnsmasq_dhcp", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			// debian defaults
			LeasesPath: "/var/lib/misc/dnsmasq.leases",
			ConfPath:   "/etc/dnsmasq.conf",
			ConfDir:    "/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new",
		},
		charts:           charts.Copy(),
		parseConfigEvery: time.Minute,
		cacheDHCPRanges:  make(map[string]bool),
		mx:               make(map[string]int64),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	LeasesPath  string `yaml:"leases_path" json:"leases_path"`
	ConfPath    string `yaml:"conf_path,omitempty" json:"conf_path"`
	ConfDir     string `yaml:"conf_dir,omitempty" json:"conf_dir"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	leasesModTime    time.Time
	parseConfigTime  time.Time
	parseConfigEvery time.Duration
	dhcpRanges       []iprange.Range
	dhcpHosts        []net.IP
	cacheDHCPRanges  map[string]bool

	mx map[string]int64
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}
	if err := c.checkLeasesPath(); err != nil {
		return fmt.Errorf("leases path check: %v", err)
	}

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
