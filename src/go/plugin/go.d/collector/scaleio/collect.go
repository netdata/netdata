// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/scaleio/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

const discoveryEvery = 5

func (s *ScaleIO) collect() (map[string]int64, error) {
	s.runs += 1
	if !s.lastDiscoveryOK || s.runs%discoveryEvery == 0 {
		if err := s.discovery(); err != nil {
			return nil, err
		}
	}

	stats, err := s.client.SelectedStatistics(query)
	if err != nil {
		return nil, err
	}

	mx := metrics{
		System:      s.collectSystem(stats.System),
		StoragePool: s.collectStoragePool(stats.StoragePool),
		Sdc:         s.collectSdc(stats.Sdc),
	}

	s.updateCharts()
	return stm.ToMap(mx), nil
}

func (s *ScaleIO) discovery() error {
	start := time.Now()
	s.Debugf("starting discovery")
	ins, err := s.client.Instances()
	if err != nil {
		s.lastDiscoveryOK = false
		return err
	}
	s.Debugf("discovering: discovered %d storage pools, %d sdcs, it took %s",
		len(ins.StoragePoolList), len(ins.SdcList), time.Since(start))

	s.discovered.pool = make(map[string]client.StoragePool, len(ins.StoragePoolList))
	for _, pool := range ins.StoragePoolList {
		s.discovered.pool[pool.ID] = pool
	}
	s.discovered.sdc = make(map[string]client.Sdc, len(ins.SdcList))
	for _, sdc := range ins.SdcList {
		s.discovered.sdc[sdc.ID] = sdc
	}
	s.lastDiscoveryOK = true
	return nil
}
