// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("isc_dhcpd", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			LeasesPath: "/var/lib/dhcp/dhcpd.leases",
		},

		collected: make(map[string]int64),
	}
}

type (
	Config struct {
		UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
		LeasesPath  string `yaml:"leases_path" json:"leases_path"`
		// TODO: parse config file to extract configured pool
		Pools []PoolConfig `yaml:"pools" json:"pools"`
	}
	PoolConfig struct {
		Name     string `yaml:"name" json:"name"`
		Networks string `yaml:"networks" json:"networks"`
	}
)

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	pools         []ipPool
	leasesModTime time.Time
	collected     map[string]int64
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	pools, err := c.initPools()
	if err != nil {
		return fmt.Errorf("ip pools init: %v", err)
	}
	c.pools = pools

	charts, err := c.initCharts(pools)
	if err != nil {
		return fmt.Errorf("charts init: %v", err)
	}
	c.charts = charts

	c.Debugf("monitoring leases file: %v", c.Config.LeasesPath)
	c.Debugf("monitoring ip pools: %v", c.Config.Pools)

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

func (c *Collector) Charts() *collectorapi.Charts {
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
