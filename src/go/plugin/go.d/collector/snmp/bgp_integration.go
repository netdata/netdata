// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type bgpIntegration struct {
	peerCache *bgpPeerCache
}

func newBGPIntegration() *bgpIntegration {
	return &bgpIntegration{
		peerCache: newBGPPeerCache(),
	}
}

func (c *Collector) prepareProfileMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	if c.bgp == nil {
		return flattenProfileMetrics(pms)
	}
	return c.bgp.prepareProfileMetrics(pms)
}

func (b *bgpIntegration) prepareProfileMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	b.peerCache.reset()

	metrics := normalizeCollectorMetrics(pms)
	for _, metric := range metrics {
		b.peerCache.updateEntry(metric)
	}

	return filterChartMetrics(metrics)
}

func (c *Collector) finalizeProfileMetrics() {
	if c.bgp == nil {
		return
	}
	c.bgp.finalizeProfileMetrics()
}

func (b *bgpIntegration) finalizeProfileMetrics() {
	b.peerCache.finalize()
}

func flattenProfileMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	var metrics []ddsnmp.Metric
	for _, pm := range pms {
		for _, metric := range pm.Metrics {
			if metric.Profile == nil {
				metric.Profile = pm
			}
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

func (c *Collector) additionalFuncHandlers() []registeredSNMPFunction {
	if c.bgp == nil {
		return nil
	}
	return c.bgp.functionHandlers()
}

func (b *bgpIntegration) functionHandlers() []registeredSNMPFunction {
	if b.peerCache == nil {
		return nil
	}
	return []registeredSNMPFunction{{
		methodID: bgpPeersMethodID,
		handler:  newFuncBGPPeers(b.peerCache),
	}}
}

func collectorSpecificMethodConfigs() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		bgpPeersMethodConfig(),
	}
}
