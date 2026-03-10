// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectFileSystems(mx *metrics) {
	for id := range c.discovered.fileSystems {
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

		mx.FileSystem[id] = fm
	}
}
