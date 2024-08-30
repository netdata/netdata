// SPDX-License-Identifier: GPL-3.0-or-later

package isc_dhcpd

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("isc_dhcpd", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *DHCPd {
	return &DHCPd{
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

type DHCPd struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	pools         []ipPool
	leasesModTime time.Time
	collected     map[string]int64
}

func (d *DHCPd) Configuration() any {
	return d.Config
}

func (d *DHCPd) Init() error {
	err := d.validateConfig()
	if err != nil {
		d.Errorf("config validation: %v", err)
		return err
	}

	pools, err := d.initPools()
	if err != nil {
		d.Errorf("ip pools init: %v", err)
		return err
	}
	d.pools = pools

	charts, err := d.initCharts(pools)
	if err != nil {
		d.Errorf("charts init: %v", err)
		return err
	}
	d.charts = charts

	d.Debugf("monitoring leases file: %v", d.Config.LeasesPath)
	d.Debugf("monitoring ip pools: %v", d.Config.Pools)

	return nil
}

func (d *DHCPd) Check() error {
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

func (d *DHCPd) Charts() *module.Charts {
	return d.charts
}

func (d *DHCPd) Collect() map[string]int64 {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (d *DHCPd) Cleanup() {}
