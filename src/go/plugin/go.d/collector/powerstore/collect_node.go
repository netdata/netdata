// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectNodes() {
	var wg sync.WaitGroup

	for id, node := range c.discovered.nodes {
		wg.Add(1)
		go func(id, name string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			pm, err := c.client.PerformanceMetricsByNode(id)
			if err != nil {
				c.Warningf("error collecting node %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.node.perf.readIops.WithLabelValues(name).Observe(last.ReadIops)
				c.mx.node.perf.writeIops.WithLabelValues(name).Observe(last.WriteIops)
				c.mx.node.perf.readBandwidth.WithLabelValues(name).Observe(last.ReadBandwidth)
				c.mx.node.perf.writeBandwidth.WithLabelValues(name).Observe(last.WriteBandwidth)
				c.mx.node.perf.avgReadLatency.WithLabelValues(name).Observe(last.AvgReadLatency)
				c.mx.node.perf.avgWriteLatency.WithLabelValues(name).Observe(last.AvgWriteLatency)
				c.mx.node.perf.avgLatency.WithLabelValues(name).Observe(last.AvgLatency)
				if last.CurrentLogins != nil {
					c.mx.node.currentLogins.WithLabelValues(name).Observe(float64(*last.CurrentLogins))
				}
			}
		}(id, node.Name)
	}

	wg.Wait()
}
