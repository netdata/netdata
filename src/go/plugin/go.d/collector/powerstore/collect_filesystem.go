// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectFileSystems() {
	var wg sync.WaitGroup

	for id, fs := range c.discovered.fileSystems {
		wg.Add(1)
		go func(id, name string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			pm, err := c.client.PerformanceMetricsByFileSystem(id)
			if err != nil {
				c.Warningf("error collecting filesystem %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.fileSystem.perf.readIops.WithLabelValues(name).Observe(last.ReadIops)
				c.mx.fileSystem.perf.writeIops.WithLabelValues(name).Observe(last.WriteIops)
				c.mx.fileSystem.perf.readBandwidth.WithLabelValues(name).Observe(last.ReadBandwidth)
				c.mx.fileSystem.perf.writeBandwidth.WithLabelValues(name).Observe(last.WriteBandwidth)
				c.mx.fileSystem.perf.avgReadLatency.WithLabelValues(name).Observe(last.AvgReadLatency)
				c.mx.fileSystem.perf.avgWriteLatency.WithLabelValues(name).Observe(last.AvgWriteLatency)
				c.mx.fileSystem.perf.avgLatency.WithLabelValues(name).Observe(last.AvgLatency)
			}
		}(id, fs.Name)
	}

	wg.Wait()
}
