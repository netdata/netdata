// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectVolumes() {
	var wg sync.WaitGroup

	for id, vol := range c.discovered.volumes {
		wg.Add(1)
		go func(id, name string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			pm, err := c.client.PerformanceMetricsByVolume(id)
			if err != nil {
				c.Warningf("error collecting volume %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.volume.perf.readIops.WithLabelValues(name).Observe(last.ReadIops)
				c.mx.volume.perf.writeIops.WithLabelValues(name).Observe(last.WriteIops)
				c.mx.volume.perf.readBandwidth.WithLabelValues(name).Observe(last.ReadBandwidth)
				c.mx.volume.perf.writeBandwidth.WithLabelValues(name).Observe(last.WriteBandwidth)
				c.mx.volume.perf.avgReadLatency.WithLabelValues(name).Observe(last.AvgReadLatency)
				c.mx.volume.perf.avgWriteLatency.WithLabelValues(name).Observe(last.AvgWriteLatency)
				c.mx.volume.perf.avgLatency.WithLabelValues(name).Observe(last.AvgLatency)
			}

			sm, err := c.client.SpaceMetricsByVolume(id)
			if err != nil {
				c.Warningf("error collecting volume %s space metrics: %v", id, err)
			} else if len(sm) > 0 {
				last := sm[len(sm)-1]
				if last.LogicalProvisioned != nil {
					c.mx.volume.spaceLogicalProv.WithLabelValues(name).Observe(float64(*last.LogicalProvisioned))
				}
				if last.LogicalUsed != nil {
					c.mx.volume.spaceLogicalUsed.WithLabelValues(name).Observe(float64(*last.LogicalUsed))
				}
				c.mx.volume.spaceThinSavings.WithLabelValues(name).Observe(last.ThinSavings)
			}
		}(id, vol.Name)
	}

	wg.Wait()
}
