// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectAppliances() {
	var wg sync.WaitGroup

	for id, app := range c.discovered.appliances {
		wg.Add(1)
		go func(id, name string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			pm, err := c.client.PerformanceMetricsByAppliance(id)
			if err != nil {
				c.Warningf("error collecting appliance %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.appliance.perf.readIops.WithLabelValues(name).Observe(last.ReadIops)
				c.mx.appliance.perf.writeIops.WithLabelValues(name).Observe(last.WriteIops)
				c.mx.appliance.perf.readBandwidth.WithLabelValues(name).Observe(last.ReadBandwidth)
				c.mx.appliance.perf.writeBandwidth.WithLabelValues(name).Observe(last.WriteBandwidth)
				c.mx.appliance.perf.avgReadLatency.WithLabelValues(name).Observe(last.AvgReadLatency)
				c.mx.appliance.perf.avgWriteLatency.WithLabelValues(name).Observe(last.AvgWriteLatency)
				c.mx.appliance.perf.avgLatency.WithLabelValues(name).Observe(last.AvgLatency)
				c.mx.appliance.cpu.WithLabelValues(name).Observe(last.IoWorkloadCPUUtilization)
			}

			sm, err := c.client.SpaceMetricsByAppliance(id)
			if err != nil {
				c.Warningf("error collecting appliance %s space metrics: %v", id, err)
			} else if len(sm) > 0 {
				last := sm[len(sm)-1]
				if last.PhysicalTotal != nil {
					c.mx.appliance.space.physicalTotal.WithLabelValues(name).Observe(float64(*last.PhysicalTotal))
				}
				if last.PhysicalUsed != nil {
					c.mx.appliance.space.physicalUsed.WithLabelValues(name).Observe(float64(*last.PhysicalUsed))
				}
				if last.LogicalProvisioned != nil {
					c.mx.appliance.space.logicalProvisioned.WithLabelValues(name).Observe(float64(*last.LogicalProvisioned))
				}
				if last.LogicalUsed != nil {
					c.mx.appliance.space.logicalUsed.WithLabelValues(name).Observe(float64(*last.LogicalUsed))
				}
				if last.DataPhysicalUsed != nil {
					c.mx.appliance.space.dataPhysicalUsed.WithLabelValues(name).Observe(float64(*last.DataPhysicalUsed))
				}
				if last.SharedLogicalUsed != nil {
					c.mx.appliance.space.sharedLogicalUsed.WithLabelValues(name).Observe(float64(*last.SharedLogicalUsed))
				}
				c.mx.appliance.space.efficiencyRatio.WithLabelValues(name).Observe(last.EfficiencyRatio)
				c.mx.appliance.space.dataReduction.WithLabelValues(name).Observe(last.DataReduction)
				c.mx.appliance.space.snapshotSavings.WithLabelValues(name).Observe(last.SnapshotSavings)
				c.mx.appliance.space.thinSavings.WithLabelValues(name).Observe(last.ThinSavings)
			}
		}(id, app.Name)
	}

	wg.Wait()
}
