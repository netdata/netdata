// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type trafficMetricWriters struct {
	bytesUp         metrix.SnapshotGaugeVec
	bytesDown       metrix.SnapshotGaugeVec
	lostUp          metrix.SnapshotGaugeVec
	lostDown        metrix.SnapshotGaugeVec
	jitterUp        metrix.SnapshotGaugeVec
	jitterDown      metrix.SnapshotGaugeVec
	discardUp       metrix.SnapshotGaugeVec
	discardDown     metrix.SnapshotGaugeVec
	rtt             metrix.SnapshotGaugeVec
	lastMileLatency metrix.SnapshotGaugeVec
	lastMileLoss    metrix.SnapshotGaugeVec
}

func (c *Collector) writeMetrics(sites map[string]*siteState, order []string, events []eventCount) {
	siteVec := c.store.Write().SnapshotMeter("").Vec("site_id", "site_name", "pop_name")
	siteConnected := siteVec.Gauge("site_connectivity_connected")
	siteDisconnected := siteVec.Gauge("site_connectivity_disconnected")
	siteDegraded := siteVec.Gauge("site_connectivity_degraded")
	siteUnknown := siteVec.Gauge("site_connectivity_unknown")
	siteActive := siteVec.Gauge("site_operational_active")
	siteDisabled := siteVec.Gauge("site_operational_disabled")
	siteLocked := siteVec.Gauge("site_operational_locked")
	siteOperationalUnknown := siteVec.Gauge("site_operational_unknown")
	siteHosts := siteVec.Gauge("site_hosts")
	siteBytesUp := siteVec.Gauge("site_bytes_upstream_max")
	siteBytesDown := siteVec.Gauge("site_bytes_downstream_max")
	siteLostUp := siteVec.Gauge("site_lost_upstream_percent")
	siteLostDown := siteVec.Gauge("site_lost_downstream_percent")
	siteJitterUp := siteVec.Gauge("site_jitter_upstream_ms")
	siteJitterDown := siteVec.Gauge("site_jitter_downstream_ms")
	siteDiscardUp := siteVec.Gauge("site_packets_discarded_upstream")
	siteDiscardDown := siteVec.Gauge("site_packets_discarded_downstream")
	siteRTT := siteVec.Gauge("site_rtt_ms")
	siteLastMileLatency := siteVec.Gauge("site_last_mile_latency_ms")
	siteLastMileLoss := siteVec.Gauge("site_last_mile_packet_loss_percent")
	siteTraffic := trafficMetricWriters{
		bytesUp:         siteBytesUp,
		bytesDown:       siteBytesDown,
		lostUp:          siteLostUp,
		lostDown:        siteLostDown,
		jitterUp:        siteJitterUp,
		jitterDown:      siteJitterDown,
		discardUp:       siteDiscardUp,
		discardDown:     siteDiscardDown,
		rtt:             siteRTT,
		lastMileLatency: siteLastMileLatency,
		lastMileLoss:    siteLastMileLoss,
	}

	ifaceVec := c.store.Write().SnapshotMeter("").Vec("site_id", "site_name", "interface_id", "interface_name")
	ifaceConnected := ifaceVec.Gauge("interface_connected")
	ifaceBytesUp := ifaceVec.Gauge("interface_bytes_upstream_max")
	ifaceBytesDown := ifaceVec.Gauge("interface_bytes_downstream_max")
	ifaceLostUp := ifaceVec.Gauge("interface_lost_upstream_percent")
	ifaceLostDown := ifaceVec.Gauge("interface_lost_downstream_percent")
	ifaceJitterUp := ifaceVec.Gauge("interface_jitter_upstream_ms")
	ifaceJitterDown := ifaceVec.Gauge("interface_jitter_downstream_ms")
	ifaceDiscardUp := ifaceVec.Gauge("interface_packets_discarded_upstream")
	ifaceDiscardDown := ifaceVec.Gauge("interface_packets_discarded_downstream")
	ifaceRTT := ifaceVec.Gauge("interface_rtt_ms")
	ifaceTunnelUptime := ifaceVec.Gauge("interface_tunnel_uptime_seconds")
	ifaceTraffic := trafficMetricWriters{
		bytesUp:     ifaceBytesUp,
		bytesDown:   ifaceBytesDown,
		lostUp:      ifaceLostUp,
		lostDown:    ifaceLostDown,
		jitterUp:    ifaceJitterUp,
		jitterDown:  ifaceJitterDown,
		discardUp:   ifaceDiscardUp,
		discardDown: ifaceDiscardDown,
		rtt:         ifaceRTT,
	}

	bgpVec := c.store.Write().SnapshotMeter("").Vec("site_id", "site_name", "peer_ip", "peer_asn")
	bgpSessionUp := bgpVec.Gauge("bgp_session_up")
	bgpRoutes := bgpVec.Gauge("bgp_routes")
	bgpRoutesLimit := bgpVec.Gauge("bgp_routes_limit")
	bgpRoutesLimitExceeded := bgpVec.Gauge("bgp_routes_limit_exceeded")
	bgpRIBOutRoutes := bgpVec.Gauge("bgp_rib_out_routes")

	for _, siteID := range order {
		site := sites[siteID]
		if site == nil {
			continue
		}
		labels := []string{site.ID, site.Name, site.PopName}

		connected, disconnected, degraded, unknownConnectivity := c.siteConnectivityValues(site.ConnectivityStatus)
		siteConnected.WithLabelValues(labels...).Observe(connected)
		siteDisconnected.WithLabelValues(labels...).Observe(disconnected)
		siteDegraded.WithLabelValues(labels...).Observe(degraded)
		siteUnknown.WithLabelValues(labels...).Observe(unknownConnectivity)
		active, disabled, locked, unknownOperational := c.siteOperationalValues(site.OperationalStatus)
		siteActive.WithLabelValues(labels...).Observe(active)
		siteDisabled.WithLabelValues(labels...).Observe(disabled)
		siteLocked.WithLabelValues(labels...).Observe(locked)
		siteOperationalUnknown.WithLabelValues(labels...).Observe(unknownOperational)
		siteHosts.WithLabelValues(labels...).Observe(float64(site.HostCount))
		writeTrafficMetrics(site.Metrics, labels, siteTraffic)

		for _, iface := range site.Interfaces {
			ifaceLabels := []string{site.ID, site.Name, iface.ID, iface.Name}
			ifaceConnected.WithLabelValues(ifaceLabels...).Observe(boolFloat(iface.Connected || iface.LinkUp))
			writeTrafficMetrics(iface.Metrics, ifaceLabels, ifaceTraffic)
			ifaceTunnelUptime.WithLabelValues(ifaceLabels...).Observe(float64(iface.TunnelUptime))
		}

		for _, peer := range site.BGPPeers {
			peerLabels := []string{site.ID, site.Name, peer.RemoteIP, peer.RemoteASN}
			bgpSessionUp.WithLabelValues(peerLabels...).Observe(boolFloat(isBGPSessionUp(peer.BGPSession)))
			bgpRoutes.WithLabelValues(peerLabels...).Observe(float64(peer.RoutesCount))
			bgpRoutesLimit.WithLabelValues(peerLabels...).Observe(float64(peer.RoutesCountLimit))
			bgpRoutesLimitExceeded.WithLabelValues(peerLabels...).Observe(boolFloat(peer.RoutesCountLimitExceeded))
			bgpRIBOutRoutes.WithLabelValues(peerLabels...).Observe(float64(peer.RIBOutRoutes))
		}
	}

	if len(events) > 0 {
		eventCounter := c.store.Write().StatefulMeter("").Vec("event_type", "event_sub_type", "severity", "status").Counter("events_total")
		for _, event := range events {
			eventCounter.WithLabelValues(event.EventType, event.EventSubType, event.Severity, event.Status).Add(float64(event.Count))
		}
	}

	if provider, ok := c.client.(apiStatsProvider); ok {
		writeAPIStats(c.store, provider.APIStats())
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

func writeAPIStats(store metrix.CollectorStore, stats apiStats) {
	if len(stats.Retries) == 0 {
		return
	}

	apiVec := store.Write().SnapshotMeter("").Vec("query")
	apiRateLimitRetries := apiVec.Counter("api_rate_limit_retries_total")
	apiTransientRetries := apiVec.Counter("api_transient_retries_total")
	for query, retries := range stats.Retries {
		apiRateLimitRetries.WithLabelValues(query).ObserveTotal(float64(retries.RateLimit))
		apiTransientRetries.WithLabelValues(query).ObserveTotal(float64(retries.Transient))
	}
}

func (c *Collector) writeCollectorHealth() {
	c.ensureHealth()

	meter := c.store.Write().SnapshotMeter("")
	meter.Gauge("collector_collection_success").Observe(boolFloat(c.health.CollectionSuccess))
	meter.Gauge("collector_discovered_sites").Observe(float64(c.health.DiscoveredSites))
	writeEntitySelectionHealth(meter, c.health)
	meter.Gauge("collector_events_marker_persistence_available").Observe(boolFloat(c.health.MarkerPersistenceAvailable))
	if c.bgpEnabled() {
		meter.Gauge("collector_bgp_sites_per_collection").Observe(float64(c.health.BGPSitesPerCollection))
		meter.Gauge("collector_bgp_full_scan_seconds").Observe(float64(c.health.BGPFullScanSeconds))
		meter.Gauge("collector_bgp_cached_sites").Observe(float64(c.health.BGPCachedSites))
	}

	if len(c.health.LastOperations) > 0 {
		operationStatus := meter.Vec("operation").Gauge("collector_operation_success")
		for _, operation := range sortedOperationNames(c.health.LastOperations) {
			status := c.health.LastOperations[operation]
			operationStatus.WithLabelValues(operation).Observe(boolFloat(status.Success))
		}
	}

	if len(c.health.OperationFailures) > 0 {
		failures := meter.Vec("operation", "error_class").Counter("collector_operation_failures_total")
		for _, key := range sortedOperationFailureKeys(c.health.OperationFailures) {
			failures.WithLabelValues(key.Operation, key.ErrorClass).ObserveTotal(float64(c.health.OperationFailures[key]))
		}
	}

	if len(c.health.OperationAffectedSites) > 0 {
		affectedSites := meter.Vec("operation", "error_class").Counter("collector_operation_affected_sites_total")
		for _, key := range sortedOperationFailureKeys(c.health.OperationAffectedSites) {
			affectedSites.WithLabelValues(key.Operation, key.ErrorClass).ObserveTotal(float64(c.health.OperationAffectedSites[key]))
		}
	}

	if len(c.health.CollectionFailureTotals) > 0 {
		failures := meter.Vec("error_class").Counter("collector_collection_failures_total")
		for _, class := range sortedStringKeys(c.health.CollectionFailureTotals) {
			failures.WithLabelValues(class).ObserveTotal(float64(c.health.CollectionFailureTotals[class]))
		}
	}

	if len(c.health.NormalizationIssues) > 0 {
		issues := meter.Vec("surface", "issue").Counter("collector_normalization_issues_total")
		for _, key := range sortedNormalizationIssueKeys(c.health.NormalizationIssues) {
			issues.WithLabelValues(key.Surface, key.Issue).ObserveTotal(float64(c.health.NormalizationIssues[key]))
		}
	}
}

func writeEntitySelectionHealth(meter metrix.SnapshotMeter, health collectorHealth) {
	selected := meter.Vec("entity").Gauge("collector_selected_entities")
	limitHit := meter.Vec("entity").Gauge("collector_cardinality_limit_hit")
	skipped := meter.Vec("entity", "reason").Gauge("collector_skipped_entities")

	for _, entity := range []string{selectionEntitySite, selectionEntityInterface, selectionEntityBGPPeer} {
		selected.WithLabelValues(entity).Observe(float64(health.SelectedEntities[entity]))
		limitHit.WithLabelValues(entity).Observe(float64(health.CardinalityLimitHits[entity]))
		for _, reason := range []string{selectionSkipSelector, selectionSkipLimit} {
			skipped.WithLabelValues(entity, reason).Observe(float64(health.SkippedEntities[entitySkipKey{Entity: entity, Reason: reason}]))
		}
	}
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
