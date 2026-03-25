// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

func (c *Collector) collectControllerStats() {
	stats, err := c.client.ControllerStatistics()
	if err != nil {
		c.Warningf("error collecting controller statistics: %v", err)
		return
	}

	for _, s := range stats {
		id := s.DurableID
		c.mx.controller.iops.WithLabelValues(id).Observe(float64(s.IOPS))
		c.mx.controller.throughput.WithLabelValues(id).Observe(float64(s.BytesPerSecond))
		c.mx.controller.cpuLoad.WithLabelValues(id).Observe(s.CPULoad)
		c.mx.controller.writeCacheUsed.WithLabelValues(id).Observe(float64(s.WriteCacheUsed))
		c.mx.controller.forwardedCmds.WithLabelValues(id).Observe(float64(s.ForwardedCmds))
		c.mx.controller.dataRead.WithLabelValues(id).Observe(float64(s.DataRead))
		c.mx.controller.dataWritten.WithLabelValues(id).Observe(float64(s.DataWritten))
		c.mx.controller.readOps.WithLabelValues(id).Observe(float64(s.NumReads))
		c.mx.controller.writeOps.WithLabelValues(id).Observe(float64(s.NumWrites))
		c.mx.controller.writeCacheHits.WithLabelValues(id).Observe(float64(s.WriteCacheHits))
		c.mx.controller.writeCacheMisses.WithLabelValues(id).Observe(float64(s.WriteCacheMisses))
		c.mx.controller.readCacheHits.WithLabelValues(id).Observe(float64(s.ReadCacheHits))
		c.mx.controller.readCacheMisses.WithLabelValues(id).Observe(float64(s.ReadCacheMisses))
	}
}
