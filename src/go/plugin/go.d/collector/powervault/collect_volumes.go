// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

func (c *Collector) collectVolumeStats() {
	stats, err := c.client.VolumeStatistics()
	if err != nil {
		c.Warningf("error collecting volume statistics: %v", err)
		return
	}

	for _, s := range stats {
		name := s.VolumeName
		if !c.volumeMatches(name) {
			continue
		}
		c.mx.volume.iops.WithLabelValues(name).Observe(float64(s.IOPS))
		c.mx.volume.throughput.WithLabelValues(name).Observe(float64(s.BytesPerSecond))
		c.mx.volume.writeCachePercent.WithLabelValues(name).Observe(float64(s.WriteCachePercent))
		c.mx.volume.dataRead.WithLabelValues(name).Observe(float64(s.DataRead))
		c.mx.volume.dataWritten.WithLabelValues(name).Observe(float64(s.DataWritten))
		c.mx.volume.readOps.WithLabelValues(name).Observe(float64(s.NumReads))
		c.mx.volume.writeOps.WithLabelValues(name).Observe(float64(s.NumWrites))
		c.mx.volume.writeCacheHits.WithLabelValues(name).Observe(float64(s.WriteCacheHits))
		c.mx.volume.writeCacheMisses.WithLabelValues(name).Observe(float64(s.WriteCacheMisses))
		c.mx.volume.readCacheHits.WithLabelValues(name).Observe(float64(s.ReadCacheHits))
		c.mx.volume.readCacheMisses.WithLabelValues(name).Observe(float64(s.ReadCacheMisses))
		c.mx.volume.tierSSD.WithLabelValues(name).Observe(float64(s.TierSSD))
		c.mx.volume.tierSAS.WithLabelValues(name).Observe(float64(s.TierSAS))
		c.mx.volume.tierSATA.WithLabelValues(name).Observe(float64(s.TierSATA))
	}
}
