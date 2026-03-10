// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectFileSystems(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id := range c.discovered.fileSystems {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			fm := fileSystemMetrics{}

			pm, err := c.client.PerformanceMetricsByFileSystem(id)
			if err != nil {
				c.Warningf("error collecting filesystem %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				fm.Perf.ReadIops = last.ReadIops
				fm.Perf.WriteIops = last.WriteIops
				fm.Perf.TotalIops = last.TotalIops
				fm.Perf.ReadBandwidth = last.ReadBandwidth
				fm.Perf.WriteBandwidth = last.WriteBandwidth
				fm.Perf.TotalBandwidth = last.TotalBandwidth
				fm.Perf.AvgReadLatency = last.AvgReadLatency
				fm.Perf.AvgWriteLatency = last.AvgWriteLatency
				fm.Perf.AvgLatency = last.AvgLatency
			}

			mu.Lock()
			mx.FileSystem[id] = fm
			mu.Unlock()
		}(id)
	}

	wg.Wait()
}
