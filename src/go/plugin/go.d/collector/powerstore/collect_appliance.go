// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectAppliances(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id := range c.discovered.appliances {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			am := applianceMetrics{}

			pm, err := c.client.PerformanceMetricsByAppliance(id)
			if err != nil {
				c.Warningf("error collecting appliance %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				am.Perf.ReadIops = last.ReadIops
				am.Perf.WriteIops = last.WriteIops
				am.Perf.TotalIops = last.TotalIops
				am.Perf.ReadBandwidth = last.ReadBandwidth
				am.Perf.WriteBandwidth = last.WriteBandwidth
				am.Perf.TotalBandwidth = last.TotalBandwidth
				am.Perf.AvgReadLatency = last.AvgReadLatency
				am.Perf.AvgWriteLatency = last.AvgWriteLatency
				am.Perf.AvgLatency = last.AvgLatency
				am.CPU = last.IoWorkloadCPUUtilization
			}

			sm, err := c.client.SpaceMetricsByAppliance(id)
			if err != nil {
				c.Warningf("error collecting appliance %s space metrics: %v", id, err)
			} else if len(sm) > 0 {
				last := sm[len(sm)-1]
				if last.PhysicalTotal != nil {
					am.Space.PhysicalTotal = *last.PhysicalTotal
				}
				if last.PhysicalUsed != nil {
					am.Space.PhysicalUsed = *last.PhysicalUsed
				}
				if last.LogicalProvisioned != nil {
					am.Space.LogicalProvisioned = *last.LogicalProvisioned
				}
				if last.LogicalUsed != nil {
					am.Space.LogicalUsed = *last.LogicalUsed
				}
				if last.DataPhysicalUsed != nil {
					am.Space.DataPhysicalUsed = *last.DataPhysicalUsed
				}
				if last.SharedLogicalUsed != nil {
					am.Space.SharedLogicalUsed = *last.SharedLogicalUsed
				}
				am.Space.EfficiencyRatio = last.EfficiencyRatio
				am.Space.DataReduction = last.DataReduction
				am.Space.SnapshotSavings = last.SnapshotSavings
				am.Space.ThinSavings = last.ThinSavings
			}

			mu.Lock()
			mx.Appliance[id] = am
			mu.Unlock()
		}(id)
	}

	wg.Wait()
}
