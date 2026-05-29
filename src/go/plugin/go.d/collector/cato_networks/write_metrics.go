// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"sort"
	"strings"
)

func (c *Collector) writeMetrics(sites map[string]*siteState, order []string) {
	for _, siteID := range order {
		site := sites[siteID]
		if site == nil {
			continue
		}
		labels := []string{site.ID, site.Name, site.PopName}

		connected, disconnected, degraded, unknownConnectivity := c.siteConnectivityValues(site.ConnectivityStatus)
		c.metrics.site.connectivityConnected.WithLabelValues(labels...).Observe(connected)
		c.metrics.site.connectivityDisconnected.WithLabelValues(labels...).Observe(disconnected)
		c.metrics.site.connectivityDegraded.WithLabelValues(labels...).Observe(degraded)
		c.metrics.site.connectivityUnknown.WithLabelValues(labels...).Observe(unknownConnectivity)
		active, disabled, locked, unknownOperational := c.siteOperationalValues(site.OperationalStatus)
		c.metrics.site.operationalActive.WithLabelValues(labels...).Observe(active)
		c.metrics.site.operationalDisabled.WithLabelValues(labels...).Observe(disabled)
		c.metrics.site.operationalLocked.WithLabelValues(labels...).Observe(locked)
		c.metrics.site.operationalUnknown.WithLabelValues(labels...).Observe(unknownOperational)
		c.metrics.site.hosts.WithLabelValues(labels...).Observe(float64(site.HostCount))
		writeTrafficMetrics(site.Metrics, labels, c.metrics.site.traffic)

		for _, iface := range site.Interfaces {
			ifaceLabels := []string{site.ID, site.Name, iface.ID, iface.Name}
			c.metrics.iface.connected.WithLabelValues(ifaceLabels...).Observe(boolFloat(iface.Connected || iface.LinkUp))
			writeTrafficMetrics(iface.Metrics, ifaceLabels, c.metrics.iface.traffic)
			c.metrics.iface.tunnelUptime.WithLabelValues(ifaceLabels...).Observe(float64(iface.TunnelUptime))
		}

		for _, peer := range site.BGPPeers {
			peerLabels := []string{site.ID, site.Name, peer.RemoteIP, peer.RemoteASN}
			c.metrics.bgp.sessionUp.WithLabelValues(peerLabels...).Observe(boolFloat(isBGPSessionUp(peer.BGPSession)))
			c.metrics.bgp.routes.WithLabelValues(peerLabels...).Observe(float64(peer.RoutesCount))
			c.metrics.bgp.routesLimit.WithLabelValues(peerLabels...).Observe(float64(peer.RoutesCountLimit))
			c.metrics.bgp.routesLimitExceeded.WithLabelValues(peerLabels...).Observe(boolFloat(peer.RoutesCountLimitExceeded))
			c.metrics.bgp.ribOutRoutes.WithLabelValues(peerLabels...).Observe(float64(peer.RIBOutRoutes))
		}
	}

	if c.client != nil {
		writeAPIStats(c.metrics.api, c.client.APIStats())
	}
}

func writeTrafficMetrics(m trafficMetrics, labels []string, writers trafficMetricWriters) {
	if m.has(trafficMetricBytesUpstreamMax) {
		writers.bytesUp.WithLabelValues(labels...).Observe(m.BytesUpstreamMax)
	}
	if m.has(trafficMetricBytesDownstreamMax) {
		writers.bytesDown.WithLabelValues(labels...).Observe(m.BytesDownstreamMax)
	}
	if m.has(trafficMetricLostUpstreamPercent) {
		writers.lostUp.WithLabelValues(labels...).Observe(m.LostUpstreamPercent)
	}
	if m.has(trafficMetricLostDownstreamPercent) {
		writers.lostDown.WithLabelValues(labels...).Observe(m.LostDownstreamPercent)
	}
	if m.has(trafficMetricJitterUpstreamMS) {
		writers.jitterUp.WithLabelValues(labels...).Observe(m.JitterUpstreamMS)
	}
	if m.has(trafficMetricJitterDownstreamMS) {
		writers.jitterDown.WithLabelValues(labels...).Observe(m.JitterDownstreamMS)
	}
	if m.has(trafficMetricPacketsDiscardedUpstream) {
		writers.discardUp.WithLabelValues(labels...).Observe(m.PacketsDiscardedUpstream)
	}
	if m.has(trafficMetricPacketsDiscardedDownstream) {
		writers.discardDown.WithLabelValues(labels...).Observe(m.PacketsDiscardedDownstream)
	}
	if m.has(trafficMetricRTTMS) {
		writers.rtt.WithLabelValues(labels...).Observe(m.RTTMS)
	}
	if writers.lastMileLatency != nil && m.has(trafficMetricLastMileLatencyMS) {
		writers.lastMileLatency.WithLabelValues(labels...).Observe(m.LastMileLatencyMS)
	}
	if writers.lastMileLoss != nil && m.has(trafficMetricLastMilePacketLossPercent) {
		writers.lastMileLoss.WithLabelValues(labels...).Observe(m.LastMilePacketLossPercent)
	}
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func writeAPIStats(metrics apiMetricInstruments, stats apiStats) {
	if len(stats.Retries) == 0 {
		return
	}

	for query, retries := range stats.Retries {
		metrics.rateLimitRetries.WithLabelValues(query).ObserveTotal(float64(retries.RateLimit))
		metrics.transientRetries.WithLabelValues(query).ObserveTotal(float64(retries.Transient))
	}
}

func (c *Collector) writeCollectorHealth() {
	c.ensureHealth()

	c.metrics.health.collectionSuccess.Observe(boolFloat(c.health.CollectionSuccess))
	c.metrics.health.discoveredSites.Observe(float64(c.health.DiscoveredSites))
	writeEntitySelectionHealth(c.metrics.health, c.health)
	c.metrics.health.bgpSitesPerCollection.Observe(float64(c.health.BGPSitesPerCollection))
	c.metrics.health.bgpFullScanSeconds.Observe(float64(c.health.BGPFullScanSeconds))
	c.metrics.health.bgpCachedSites.Observe(float64(c.health.BGPCachedSites))

	if len(c.health.LastOperations) > 0 {
		for _, operation := range sortedOperationNames(c.health.LastOperations) {
			status := c.health.LastOperations[operation]
			c.metrics.health.operationSuccess.WithLabelValues(operation).Observe(boolFloat(status.Success))
		}
	}

	if len(c.health.OperationFailures) > 0 {
		for _, key := range sortedOperationFailureKeys(c.health.OperationFailures) {
			c.metrics.health.operationFailures.WithLabelValues(key.Operation, key.ErrorClass).ObserveTotal(float64(c.health.OperationFailures[key]))
		}
	}

	if len(c.health.OperationAffectedSites) > 0 {
		for _, key := range sortedOperationFailureKeys(c.health.OperationAffectedSites) {
			c.metrics.health.operationAffectedSites.WithLabelValues(key.Operation, key.ErrorClass).ObserveTotal(float64(c.health.OperationAffectedSites[key]))
		}
	}

	if len(c.health.CollectionFailureTotals) > 0 {
		for _, class := range sortedStringKeys(c.health.CollectionFailureTotals) {
			c.metrics.health.collectionFailures.WithLabelValues(class).ObserveTotal(float64(c.health.CollectionFailureTotals[class]))
		}
	}

	if len(c.health.NormalizationIssues) > 0 {
		for _, key := range sortedNormalizationIssueKeys(c.health.NormalizationIssues) {
			c.metrics.health.normalizationIssues.WithLabelValues(key.Surface, key.Issue).ObserveTotal(float64(c.health.NormalizationIssues[key]))
		}
	}
}

func writeEntitySelectionHealth(metrics collectorHealthMetricInstruments, health collectorHealth) {
	metrics.selectedEntities.WithLabelValues(selectionEntitySite).Observe(float64(health.SelectedEntities[selectionEntitySite]))
	metrics.skippedEntities.WithLabelValues(selectionEntitySite, selectionSkipSelector).Observe(float64(health.SkippedEntities[entitySkipKey{Entity: selectionEntitySite, Reason: selectionSkipSelector}]))
}

func sortedOperationNames(values map[string]operationHealth) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedOperationFailureKeys(values map[operationFailureKey]int64) []operationFailureKey {
	keys := make([]operationFailureKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].ErrorClass < keys[j].ErrorClass
	})
	return keys
}

func sortedNormalizationIssueKeys(values map[normalizationIssueKey]int64) []normalizationIssueKey {
	keys := make([]normalizationIssueKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Surface != keys[j].Surface {
			return keys[i].Surface < keys[j].Surface
		}
		return keys[i].Issue < keys[j].Issue
	})
	return keys
}

func sortedStringKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isBGPSessionUp(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "up" || status == "established"
}

func (c *Collector) siteConnectivityValues(status string) (connected, disconnected, degraded, unknown float64) {
	switch strings.TrimSpace(status) {
	case "connected":
		return 1, 0, 0, 0
	case "disconnected":
		return 0, 1, 0, 0
	case "degraded":
		return 0, 0, 1, 0
	case "", "unknown":
		return 0, 0, 0, 1
	default:
		c.markNormalizationIssue(normalizationSurfaceSiteConnectivity, normalizationIssueUnknownStatus)
		return 0, 0, 0, 1
	}
}

func (c *Collector) siteOperationalValues(status string) (active, disabled, locked, unknown float64) {
	switch strings.TrimSpace(status) {
	case "active":
		return 1, 0, 0, 0
	case "disabled":
		return 0, 1, 0, 0
	case "locked":
		return 0, 0, 1, 0
	case "", "unknown":
		return 0, 0, 0, 1
	default:
		c.markNormalizationIssue(normalizationSurfaceSiteOperational, normalizationIssueUnknownStatus)
		return 0, 0, 0, 1
	}
}
