// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powerstore/client"
)

func (c *Collector) collectFcPorts(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id, port := range c.discovered.fcPorts {
		wg.Add(1)
		go func(id string, port client.FcPort) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

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

			mu.Lock()
			mx.FcPort[id] = fm
			mu.Unlock()
		}(id, port)
	}

	wg.Wait()
}

func (c *Collector) collectEthPorts(mx *metrics) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for id, port := range c.discovered.ethPorts {
		wg.Add(1)
		go func(id string, port client.EthPort) {
			defer wg.Done()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()

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

			mu.Lock()
			mx.EthPort[id] = em
			mu.Unlock()
		}(id, port)
	}

	wg.Wait()
}
