// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type bgpIntegration struct {
	peerCache            *bgpPeerCache
	collectError         error
	collectFailedSources map[string]bool
}

func newBGPIntegration() *bgpIntegration {
	return &bgpIntegration{
		peerCache: newBGPPeerCache(),
	}
}

func (c *Collector) enableBGPIntegration() {
	if c.bgp == nil {
		c.bgp = newBGPIntegration()
	}
	c.bgp.setStaleAfter(c.bgpStaleAfter())
	if c.funcRouter != nil {
		for _, h := range c.bgp.functionHandlers() {
			c.funcRouter.registerHandler(h.methodID, h.handler)
		}
	}
}

func (c *Collector) markBGPCollectFailed(err error) {
	if c.bgp == nil {
		return
	}
	c.bgp.markCollectFailed(err)
}

func (b *bgpIntegration) setStaleAfter(d time.Duration) {
	if b == nil || b.peerCache == nil {
		return
	}
	b.peerCache.setStaleAfter(d)
}

func (b *bgpIntegration) markCollectFailed(err error) {
	if b == nil || b.peerCache == nil {
		return
	}
	b.peerCache.markCollectFailed(err)
}

func (c *Collector) prepareProfileMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	if c.bgp == nil {
		return flattenProfileMetrics(pms)
	}
	return c.bgp.prepareProfileMetrics(pms)
}

func (b *bgpIntegration) prepareProfileMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	b.collectError, b.collectFailedSources = bgpCollectErrorsBySource(pms)

	metrics := flattenProfileMetrics(pms)
	b.peerCache.resetExceptSources(b.collectFailedSources)
	for _, pm := range pms {
		if pm == nil || pm.BGPCollectError != nil {
			continue
		}
		for _, row := range pm.BGPRows {
			b.peerCache.updateRow(pm.Source, row)
		}
	}

	metrics = append(metrics, typedBGPMetricsFromProfileMetrics(successfulBGPProfileMetrics(pms))...)

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
	if b.collectError != nil {
		b.peerCache.markSourcesCollectFailed(b.collectFailedSources, b.collectError)
	}
	b.collectError = nil
	b.collectFailedSources = nil
}

func bgpCollectErrorsBySource(pms []*ddsnmp.ProfileMetrics) (error, map[string]bool) {
	var errs []error
	sources := make(map[string]bool)
	for _, pm := range pms {
		if pm == nil || pm.BGPCollectError == nil {
			continue
		}
		if pm.Source == "" {
			errs = append(errs, pm.BGPCollectError)
			continue
		}
		sources[pm.Source] = true
		errs = append(errs, fmt.Errorf("%s: %w", pm.Source, pm.BGPCollectError))
	}
	if len(sources) == 0 {
		sources = nil
	}
	return errors.Join(errs...), sources
}

func successfulBGPProfileMetrics(pms []*ddsnmp.ProfileMetrics) []*ddsnmp.ProfileMetrics {
	result := make([]*ddsnmp.ProfileMetrics, 0, len(pms))
	for _, pm := range pms {
		if pm == nil || pm.BGPCollectError != nil {
			continue
		}
		result = append(result, pm)
	}
	return result
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

func collectorSpecificFunctionHandlers() []registeredSNMPFunction {
	return []registeredSNMPFunction{{
		methodID: bgpPeersMethodID,
		handler:  newFuncBGPPeers(nil),
	}}
}

func collectorSpecificMethodConfigs() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		bgpPeersMethodConfig(),
	}
}

func profilesHaveBGP(profiles []*ddsnmp.Profile) bool {
	for _, prof := range profiles {
		if profileHasBGP(prof) {
			return true
		}
	}
	return false
}

func profileHasBGP(prof *ddsnmp.Profile) bool {
	if prof == nil || prof.Definition == nil {
		return false
	}
	def := prof.Definition
	if len(def.BGP) > 0 {
		return true
	}
	return false
}

func profileMetricsHaveBGP(pms []*ddsnmp.ProfileMetrics) bool {
	for _, pm := range pms {
		if pm.BGPCollectError != nil || len(pm.BGPRows) > 0 {
			return true
		}
	}
	return false
}

func (c *Collector) bgpStaleAfter() time.Duration {
	updateEvery := c.UpdateEvery
	if updateEvery <= 0 {
		updateEvery = 10
	}
	return time.Duration(updateEvery*3) * time.Second
}
