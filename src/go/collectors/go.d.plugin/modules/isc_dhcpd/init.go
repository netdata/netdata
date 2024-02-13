// SPDX-License-Identifier: GPL-3.0-or-later

package isc_dhcpd

import (
	"errors"
	"fmt"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/iprange"
)

type ipPool struct {
	name      string
	addresses iprange.Pool
}

func (d DHCPd) validateConfig() error {
	if d.Config.LeasesPath == "" {
		return errors.New("'lease_path' parameter not set")
	}
	if len(d.Config.Pools) == 0 {
		return errors.New("'pools' parameter not set")
	}
	for i, cfg := range d.Config.Pools {
		if cfg.Name == "" {
			return fmt.Errorf("'pools[%d]->pool.name' parameter not set", i+1)
		}
		if cfg.Networks == "" {
			return fmt.Errorf("'pools[%d]->pool.networks' parameter not set", i+1)
		}
	}
	return nil
}

func (d DHCPd) initPools() ([]ipPool, error) {
	var pools []ipPool
	for i, cfg := range d.Pools {
		rs, err := iprange.ParseRanges(cfg.Networks)
		if err != nil {
			return nil, fmt.Errorf("parse pools[%d]->pool.networks '%s' ('%s'): %v", i+1, cfg.Name, cfg.Networks, err)
		}
		if len(rs) != 0 {
			pools = append(pools, ipPool{
				name:      cfg.Name,
				addresses: rs,
			})
		}
	}
	return pools, nil
}

func (d DHCPd) initCharts(pools []ipPool) (*module.Charts, error) {
	charts := &module.Charts{}

	if err := charts.Add(activeLeasesTotalChart.Copy()); err != nil {
		return nil, err
	}

	chart := poolActiveLeasesChart.Copy()
	if err := charts.Add(chart); err != nil {
		return nil, err
	}
	for _, pool := range pools {
		dim := &module.Dim{
			ID:   "pool_" + pool.name + "_active_leases",
			Name: pool.name,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}

	chart = poolUtilizationChart.Copy()
	if err := charts.Add(chart); err != nil {
		return nil, err
	}
	for _, pool := range pools {
		dim := &module.Dim{
			ID:   "pool_" + pool.name + "_utilization",
			Name: pool.name,
			Div:  precision,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}

	return charts, nil
}
