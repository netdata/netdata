// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) writeMetrics(sites map[string]*siteState, order []string) {
	for _, siteID := range order {
		site := sites[siteID]
		if site == nil {
			continue
		}
		labels := []string{site.ID, site.Name, site.PopName}

		observeStateSetVec(c.metrics.site.connectivityStatus, c.siteConnectivityState(site.ConnectivityStatus), labels...)
		observeStateSetVec(c.metrics.site.operationalStatus, c.siteOperationalState(site.OperationalStatus), labels...)
		c.metrics.site.hosts.WithLabelValues(labels...).Observe(float64(site.HostCount))
		writeTrafficMetrics(site.Metrics, labels, c.metrics.site.traffic)

		for _, iface := range site.Interfaces {
			ifaceLabels := []string{site.ID, site.Name, iface.ID, iface.Name}
			observeStateSetVec(c.metrics.iface.connectionStatus, boolState(iface.Connected || iface.LinkUp, "connected", "disconnected"), ifaceLabels...)
			writeTrafficMetrics(iface.Metrics, ifaceLabels, c.metrics.iface.traffic)
			c.metrics.iface.tunnelUptime.WithLabelValues(ifaceLabels...).Observe(float64(iface.TunnelUptime))
		}

		for _, peer := range site.BGPPeers {
			peerLabels := []string{site.ID, site.Name, peer.RemoteIP, peer.RemoteASN}
			observeStateSetVec(c.metrics.bgp.sessionStatus, bgpSessionState(peer.BGPSession), peerLabels...)
			c.metrics.bgp.routes.WithLabelValues(peerLabels...).Observe(float64(peer.RoutesCount))
			c.metrics.bgp.routesLimit.WithLabelValues(peerLabels...).Observe(float64(peer.RoutesCountLimit))
			observeStateSetVec(c.metrics.bgp.routesLimitState, boolState(peer.RoutesCountLimitExceeded, "exceeded", "ok"), peerLabels...)
			c.metrics.bgp.ribOutRoutes.WithLabelValues(peerLabels...).Observe(float64(peer.RIBOutRoutes))
		}
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

func observeStateSetVec(vec metrix.SnapshotStateSetVec, active string, labels ...string) {
	if active == "" {
		return
	}
	vec.WithLabelValues(labels...).Enable(active)
}

func boolState(ok bool, trueState, falseState string) string {
	if ok {
		return trueState
	}
	return falseState
}

func bgpSessionState(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "up", "established":
		return "up"
	case "", "unknown":
		return "unknown"
	default:
		return "down"
	}
}

func (c *Collector) siteConnectivityState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "connected":
		return "connected"
	case "disconnected":
		return "disconnected"
	case "degraded":
		return "degraded"
	case "", "unknown":
		return "unknown"
	default:
		c.logNormalizationIssue(normalizationSurfaceSiteConnectivity, normalizationIssueUnknownStatus)
		return "unknown"
	}
}

func (c *Collector) siteOperationalState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "active"
	case "disabled":
		return "disabled"
	case "locked":
		return "locked"
	case "", "unknown":
		return "unknown"
	default:
		c.logNormalizationIssue(normalizationSurfaceSiteOperational, normalizationIssueUnknownStatus)
		return "unknown"
	}
}
