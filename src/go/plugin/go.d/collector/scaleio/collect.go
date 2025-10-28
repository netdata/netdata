// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/scaleio/client"
)

const discoveryEvery = 5

func (c *Collector) collect() (map[string]int64, error) {
	c.runs += 1
	if !c.lastDiscoveryOK || c.runs%discoveryEvery == 0 {
		if err := c.discovery(); err != nil {
			return nil, err
		}
	}

	stats, err := c.client.SelectedStatistics(query)
	if err != nil {
		return nil, err
	}

	mx := metrics{
		System:      c.collectSystem(stats.System),
		StoragePool: c.collectStoragePool(stats.StoragePool),
		Sdc:         c.collectSdc(stats.Sdc),
	}

	c.updateCharts()
	return stm.ToMap(mx), nil
}

func (c *Collector) discovery() error {
	start := time.Now()
	c.Debugf("starting discovery")
	ins, err := c.client.Instances()
	if err != nil {
		c.lastDiscoveryOK = false
		return err
	}
	c.Debugf("discovering: discovered %d storage pools, %d sdcs, it took %s",
		len(ins.StoragePoolList), len(ins.SdcList), time.Since(start))

	c.discovered.pool = make(map[string]client.StoragePool, len(ins.StoragePoolList))
	for _, pool := range ins.StoragePoolList {
		c.discovered.pool[pool.ID] = pool
	}
	c.discovered.sdc = make(map[string]client.Sdc, len(ins.SdcList))
	for _, sdc := range ins.SdcList {
		c.discovered.sdc[sdc.ID] = sdc
	}
	c.lastDiscoveryOK = true
	return nil
}
