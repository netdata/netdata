// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectFcPorts(mx *metrics) {
	for id, port := range c.discovered.fcPorts {
		fm := fcPortMetrics{}
		if port.IsLinkUp {
			fm.LinkUp = 1
		}

		pm, err := c.client.PerformanceMetricsByFcPort(id)
		if err != nil {
			c.Warningf("error collecting FC port %s perf metrics: %v", id, err)
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

		mx.FcPort[id] = fm
	}
}

func (c *Collector) collectEthPorts(mx *metrics) {
	for id, port := range c.discovered.ethPorts {
		em := ethPortMetrics{}
		if port.IsLinkUp {
			em.LinkUp = 1
		}

		pm, err := c.client.EthPortPerformanceMetrics(id)
		if err != nil {
			c.Warningf("error collecting ETH port %s perf metrics: %v", id, err)
		} else if len(pm) > 0 {
			last := pm[len(pm)-1]
			em.BytesRxPs = last.BytesRxPs
			em.BytesTxPs = last.BytesTxPs
			em.PktRxPs = last.PktRxPs
			em.PktTxPs = last.PktTxPs
			em.PktRxCrcErrorPs = last.PktRxCrcErrorPs
			em.PktRxNoBufferErrorPs = last.PktRxNoBufferErrorPs
			em.PktTxErrorPs = last.PktTxErrorPs
		}

		mx.EthPort[id] = em
	}
}
