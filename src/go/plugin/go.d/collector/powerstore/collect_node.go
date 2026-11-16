// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"context"
	"sync"
)

func (c *Collector) collectNodes(ctx context.Context) {
	var wg sync.WaitGroup

	for id, node := range c.discovered.nodes {
		wg.Add(1)
		go func(id, name string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			pm, err := c.client.PerformanceMetricsByNode(ctx, id)
			if err != nil {
				c.Warningf("error collecting node %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.node.perf.readIops.WithLabelValues(name).Observe(last.AvgReadIops)
				c.mx.node.perf.writeIops.WithLabelValues(name).Observe(last.AvgWriteIops)
				c.mx.node.perf.readBandwidth.WithLabelValues(name).Observe(last.AvgReadBandwidth)
				c.mx.node.perf.writeBandwidth.WithLabelValues(name).Observe(last.AvgWriteBandwidth)
				c.mx.node.perf.avgReadLatency.WithLabelValues(name).Observe(last.AvgReadLatency)
				c.mx.node.perf.avgWriteLatency.WithLabelValues(name).Observe(last.AvgWriteLatency)
				c.mx.node.perf.avgLatency.WithLabelValues(name).Observe(last.AvgLatency)
				if last.AvgCurrentLogins != nil {
					c.mx.node.currentLogins.WithLabelValues(name).Observe(*last.AvgCurrentLogins)
				}
			}
		}(id, node.Name)
	}

	wg.Wait()
}
