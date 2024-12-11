// SPDX-License-Identifier: GPL-3.0-or-later

package geth

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func (c *Collector) collect() (map[string]int64, error) {
	pms, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}
	mx := c.collectGeth(pms)

	return stm.ToMap(mx), nil
}

func (c *Collector) collectGeth(pms prometheus.Series) map[string]float64 {
	mx := make(map[string]float64)
	c.collectChainData(mx, pms)
	c.collectP2P(mx, pms)
	c.collectTxPool(mx, pms)
	c.collectRpc(mx, pms)
	return mx
}

func (c *Collector) collectChainData(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		chainValidation,
		chainWrite,
		ethDbChainDataAncientRead,
		ethDbChainDataAncientWrite,
		ethDbChaindataDiskRead,
		ethDbChainDataDiskWrite,
		chainHeadBlock,
		chainHeadHeader,
		chainHeadReceipt,
		ethDbChainDataAncientSize,
		ethDbChainDataDiskSize,
		reorgsAdd,
		reorgsDropped,
		reorgsExecuted,
		goRoutines,
	)
	c.collectEth(mx, pms)

}

func (c *Collector) collectRpc(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		rpcRequests,
		rpcSuccess,
		rpcFailure,
	)
	c.collectEth(mx, pms)
}

func (c *Collector) collectTxPool(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		txPoolInvalid,
		txPoolPending,
		txPoolLocal,
		txPoolPendingDiscard,
		txPoolNofunds,
		txPoolPendingRatelimit,
		txPoolPendingReplace,
		txPoolQueuedDiscard,
		txPoolQueuedEviction,
		txPoolQueuedEviction,
		txPoolQueuedRatelimit,
	)
	c.collectEth(mx, pms)
}

func (c *Collector) collectP2P(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		p2pDials,
		p2pEgress,
		p2pIngress,
		p2pPeers,
		p2pServes,
	)
	c.collectEth(mx, pms)
}

func (c *Collector) collectEth(mx map[string]float64, pms prometheus.Series) {
	for _, pm := range pms {
		mx[pm.Name()] += pm.Value
	}
}
