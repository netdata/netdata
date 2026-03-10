// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "sync"

func (c *Collector) collectFcPorts() {
	var wg sync.WaitGroup

	for id, port := range c.discovered.fcPorts {
		wg.Add(1)
		go func(id, name string, linkUp bool) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			var linkVal float64
			if linkUp {
				linkVal = 1
			}
			c.mx.fcPort.linkUp.WithLabelValues(name).Observe(linkVal)

			pm, err := c.client.PerformanceMetricsByFcPort(id)
			if err != nil {
				c.Warningf("error collecting FC port %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.fcPort.perf.readIops.WithLabelValues(name).Observe(last.ReadIops)
				c.mx.fcPort.perf.writeIops.WithLabelValues(name).Observe(last.WriteIops)
				c.mx.fcPort.perf.readBandwidth.WithLabelValues(name).Observe(last.ReadBandwidth)
				c.mx.fcPort.perf.writeBandwidth.WithLabelValues(name).Observe(last.WriteBandwidth)
				c.mx.fcPort.perf.avgReadLatency.WithLabelValues(name).Observe(last.AvgReadLatency)
				c.mx.fcPort.perf.avgWriteLatency.WithLabelValues(name).Observe(last.AvgWriteLatency)
				c.mx.fcPort.perf.avgLatency.WithLabelValues(name).Observe(last.AvgLatency)
			}
		}(id, port.Name, port.IsLinkUp)
	}

	wg.Wait()
}

func (c *Collector) collectEthPorts() {
	var wg sync.WaitGroup

	for id, port := range c.discovered.ethPorts {
		wg.Add(1)
		go func(id, name string, linkUp bool) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

			var linkVal float64
			if linkUp {
				linkVal = 1
			}
			c.mx.ethPort.linkUp.WithLabelValues(name).Observe(linkVal)

			pm, err := c.client.EthPortPerformanceMetrics(id)
			if err != nil {
				c.Warningf("error collecting ETH port %s perf metrics: %v", id, err)
			} else if len(pm) > 0 {
				last := pm[len(pm)-1]
				c.mx.ethPort.bytesRxPs.WithLabelValues(name).Observe(float64(last.BytesRxPs))
				c.mx.ethPort.bytesTxPs.WithLabelValues(name).Observe(float64(last.BytesTxPs))
				c.mx.ethPort.pktRxPs.WithLabelValues(name).Observe(float64(last.PktRxPs))
				c.mx.ethPort.pktTxPs.WithLabelValues(name).Observe(float64(last.PktTxPs))
				c.mx.ethPort.pktRxCrcErrorPs.WithLabelValues(name).Observe(float64(last.PktRxCrcErrorPs))
				c.mx.ethPort.pktRxNoBufferErrorPs.WithLabelValues(name).Observe(float64(last.PktRxNoBufferErrorPs))
				c.mx.ethPort.pktTxErrorPs.WithLabelValues(name).Observe(float64(last.PktTxErrorPs))
			}
		}(id, port.Name, port.IsLinkUp)
	}

	wg.Wait()
}
