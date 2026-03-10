// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectNodes(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id := range c.discovered.nodes {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			nm := nodeMetrics{}

			pm, err := c.client.PerformanceMetricsByNode(id)
			if err != nil {
				c.Warningf("error collecting node %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				nm.Perf.ReadIops = last.ReadIops
				nm.Perf.WriteIops = last.WriteIops
				nm.Perf.TotalIops = last.TotalIops
				nm.Perf.ReadBandwidth = last.ReadBandwidth
				nm.Perf.WriteBandwidth = last.WriteBandwidth
				nm.Perf.TotalBandwidth = last.TotalBandwidth
				nm.Perf.AvgReadLatency = last.AvgReadLatency
				nm.Perf.AvgWriteLatency = last.AvgWriteLatency
				nm.Perf.AvgLatency = last.AvgLatency
				if last.CurrentLogins != nil {
					nm.CurrentLogins = *last.CurrentLogins
				}
			}

			mu.Lock()
			mx.Node[id] = nm
			mu.Unlock()
		}(id)
	}

	wg.Wait()
}
