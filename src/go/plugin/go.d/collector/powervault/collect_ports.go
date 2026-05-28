// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

func (c *Collector) collectPortStats() {
	stats, err := c.client.PortStatistics()
	if err != nil {
		c.Warningf("error collecting port statistics: %v", err)
		return
	}

	for _, s := range stats {
		id := s.DurableID
		c.mx.port.readOps.WithLabelValues(id).Observe(float64(s.NumReads))
		c.mx.port.writeOps.WithLabelValues(id).Observe(float64(s.NumWrites))
		c.mx.port.dataRead.WithLabelValues(id).Observe(float64(s.DataRead))
		c.mx.port.dataWritten.WithLabelValues(id).Observe(float64(s.DataWritten))
	}
}

func (c *Collector) collectPhyStats() {
	stats, err := c.client.PhyStatistics()
	if err != nil {
		c.Warningf("error collecting PHY statistics: %v", err)
		return
	}

	// Aggregate PHY errors per port (multiple PHYs per port).
	type portErrors struct {
		disparity, lost, invalid int64
	}
	byPort := make(map[string]*portErrors)

	for _, s := range stats {
		// Normalize to match port durable ID format (e.g. "A0" → "hostport_A0").
		portID := "hostport_" + s.Port
		pe, ok := byPort[portID]
		if !ok {
			pe = &portErrors{}
			byPort[portID] = pe
		}
		pe.disparity += s.DisparityErrors
		pe.lost += s.LostDwords
		pe.invalid += s.InvalidDwords
	}

	for portID, pe := range byPort {
		c.mx.phy.disparityErrors.WithLabelValues(portID).Observe(float64(pe.disparity))
		c.mx.phy.lostDwords.WithLabelValues(portID).Observe(float64(pe.lost))
		c.mx.phy.invalidDwords.WithLabelValues(portID).Observe(float64(pe.invalid))
	}
}
