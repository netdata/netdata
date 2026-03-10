// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectVolumes(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id := range c.discovered.volumes {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			vm := volumeMetrics{}

			pm, err := c.client.PerformanceMetricsByVolume(id)
			if err != nil {
				c.Warningf("error collecting volume %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				vm.Perf.ReadIops = last.ReadIops
				vm.Perf.WriteIops = last.WriteIops
				vm.Perf.TotalIops = last.TotalIops
				vm.Perf.ReadBandwidth = last.ReadBandwidth
				vm.Perf.WriteBandwidth = last.WriteBandwidth
				vm.Perf.TotalBandwidth = last.TotalBandwidth
				vm.Perf.AvgReadLatency = last.AvgReadLatency
				vm.Perf.AvgWriteLatency = last.AvgWriteLatency
				vm.Perf.AvgLatency = last.AvgLatency
			}

			sm, err := c.client.SpaceMetricsByVolume(id)
			if err != nil {
				c.Warningf("error collecting volume %s space metrics: %v", id, err)
			} else if len(sm) > 0 {
				last := sm[len(sm)-1]
				if last.LogicalProvisioned != nil {
					vm.Space.LogicalProvisioned = *last.LogicalProvisioned
				}
				if last.LogicalUsed != nil {
					vm.Space.LogicalUsed = *last.LogicalUsed
				}
				vm.Space.ThinSavings = last.ThinSavings
			}

			mu.Lock()
			mx.Volume[id] = vm
			mu.Unlock()
		}(id)
	}

	wg.Wait()
}
