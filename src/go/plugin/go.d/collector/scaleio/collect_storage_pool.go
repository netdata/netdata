// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/scaleio/client"

func (c *Collector) collectStoragePool(ss map[string]client.StoragePoolStatistics) map[string]storagePoolMetrics {
	ms := make(map[string]storagePoolMetrics, len(ss))

	for id, stats := range ss {
		pool, ok := c.discovered.pool[id]
		if !ok {
			continue
		}
		var pm storagePoolMetrics
		collectStoragePoolCapacity(&pm, stats, pool)
		collectStoragePoolComponents(&pm, stats)

		ms[id] = pm
	}
	return ms
}

func collectStoragePoolCapacity(pm *storagePoolMetrics, ps client.StoragePoolStatistics, pool client.StoragePool) {
	collectCapacity(&pm.Capacity.capacity, ps.CapacityStatistics)
	pm.Capacity.Utilization = calcCapacityUtilization(ps.CapacityInUseInKb, ps.MaxCapacityInKb, pool.SparePercentage)
	pm.Capacity.AlertThreshold.Critical = pool.CapacityAlertCriticalThreshold
	pm.Capacity.AlertThreshold.High = pool.CapacityAlertHighThreshold
}

func collectStoragePoolComponents(pm *storagePoolMetrics, ps client.StoragePoolStatistics) {
	pm.Components.Devices = ps.NumOfDevices
	pm.Components.Snapshots = ps.NumOfSnapshots
	pm.Components.Volumes = ps.NumOfVolumes
	pm.Components.Vtrees = ps.NumOfVtrees
}

func calcCapacityUtilization(inUse int64, max int64, sparePercent int64) float64 {
	spare := float64(max) / 100 * float64(sparePercent)
	return divFloat(float64(100*inUse), float64(max)-spare)
}
