// SPDX-License-Identifier: GPL-3.0-or-later

package geth

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func (g *Geth) collect() (map[string]int64, error) {
	pms, err := g.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}
	mx := g.collectGeth(pms)

	return stm.ToMap(mx), nil
}

func (g *Geth) collectGeth(pms prometheus.Series) map[string]float64 {
	mx := make(map[string]float64)
	g.collectChainData(mx, pms)
	g.collectP2P(mx, pms)
	g.collectTxPool(mx, pms)
	g.collectRpc(mx, pms)
	return mx
}

func (g *Geth) collectChainData(mx map[string]float64, pms prometheus.Series) {
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
	g.collectEth(mx, pms)

}

func (g *Geth) collectRpc(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		rpcRequests,
		rpcSuccess,
		rpcFailure,
	)
	g.collectEth(mx, pms)
}

func (g *Geth) collectTxPool(mx map[string]float64, pms prometheus.Series) {
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
	g.collectEth(mx, pms)
}

func (g *Geth) collectP2P(mx map[string]float64, pms prometheus.Series) {
	pms = pms.FindByNames(
		p2pDials,
		p2pEgress,
		p2pIngress,
		p2pPeers,
		p2pServes,
	)
	g.collectEth(mx, pms)
}

func (g *Geth) collectEth(mx map[string]float64, pms prometheus.Series) {
	for _, pm := range pms {
		mx[pm.Name()] += pm.Value
	}
}
