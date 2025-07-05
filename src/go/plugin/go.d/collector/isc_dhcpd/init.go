// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

type ipPool struct {
	name      string
	addresses *iprange.Pool
}

func (c *Collector) validateConfig() error {
	if c.Config.LeasesPath == "" {
		return errors.New("'lease_path' parameter not set")
	}
	if len(c.Config.Pools) == 0 {
		return errors.New("'pools' parameter not set")
	}
	for i, cfg := range c.Config.Pools {
		if cfg.Name == "" {
			return fmt.Errorf("'pools[%d]->pool.name' parameter not set", i+1)
		}
		if cfg.Networks == "" {
			return fmt.Errorf("'pools[%d]->pool.networks' parameter not set", i+1)
		}
	}
	return nil
}

func (c *Collector) initPools() ([]ipPool, error) {
	var pools []ipPool

	for i, cfg := range c.Pools {
		ipRange, err := iprange.ParseRanges(cfg.Networks)
		if err != nil {
			return nil, fmt.Errorf("parse pools[%d]->pool.networks '%s' ('%s'): %v", i+1, cfg.Name, cfg.Networks, err)
		}
		if len(ipRange) == 0 {
			continue
		}

		pool := ipPool{name: cfg.Name, addresses: iprange.NewPool(ipRange...)}
		pools = append(pools, pool)
	}

	return pools, nil
}

func (c *Collector) initCharts(pools []ipPool) (*module.Charts, error) {
	charts := &module.Charts{}

	if err := charts.Add(activeLeasesTotalChart.Copy()); err != nil {
		return nil, err
	}

	for _, pool := range pools {
		poolCharts := dhcpPoolChartsTmpl.Copy()

		for _, chart := range *poolCharts {
			chart.ID = fmt.Sprintf(chart.ID, cleanPoolNameForChart(pool.name))
			chart.Labels = []module.Label{
				{Key: "dhcp_pool_name", Value: pool.name},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, pool.name)
			}
		}

		if err := charts.Add(*poolCharts...); err != nil {
			return nil, err
		}
	}

	return charts, nil
}

func cleanPoolNameForChart(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}
