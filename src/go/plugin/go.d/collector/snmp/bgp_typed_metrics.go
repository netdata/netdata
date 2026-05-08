// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type bgpTypedMetricBuilder struct {
	metrics                  []ddsnmp.Metric
	explicitDevicePeerCounts map[string]int64
	autoDevicePeerCounts     map[string]int64
	devicePeerStates         map[string]int64
	deviceSample             *ddsnmp.ProfileMetrics
}

func typedBGPMetricsFromProfileMetrics(pms []*ddsnmp.ProfileMetrics) []ddsnmp.Metric {
	builder := &bgpTypedMetricBuilder{}
	for _, pm := range pms {
		for _, row := range pm.BGPRows {
			builder.addRow(pm, row)
		}
	}
	return builder.metricsWithDeviceSummaries()
}

func (b *bgpTypedMetricBuilder) addRow(pm *ddsnmp.ProfileMetrics, row ddsnmp.BGPRow) {
	if b.deviceSample == nil {
		b.deviceSample = pm
	}

	switch row.Kind {
	case ddprofiledefinition.BGPRowKindDevice:
		b.addDeviceRow(pm, row)
	case ddprofiledefinition.BGPRowKindPeer:
		b.addPeerLikeRow(pm, row, bgpScopePeers)
		b.addPeerDeviceSummary(row)
	case ddprofiledefinition.BGPRowKindPeerFamily:
		b.addPeerLikeRow(pm, row, bgpScopePeerFamilies)
	}
}

func (b *bgpTypedMetricBuilder) addDeviceRow(_ *ddsnmp.ProfileMetrics, row ddsnmp.BGPRow) {
	if row.Device.Peers.Has {
		b.addExplicitDevicePeerCount("configured", row.Device.Peers.Value)
	}
	if row.Device.InternalPeers.Has {
		b.addExplicitDevicePeerCount("ibgp", row.Device.InternalPeers.Value)
	}
	if row.Device.ExternalPeers.Has {
		b.addExplicitDevicePeerCount("ebgp", row.Device.ExternalPeers.Value)
	}
	if row.Device.ByStateHas {
		for state, value := range row.Device.ByState {
			b.addDevicePeerState(string(state), value)
		}
	}
}

func (b *bgpTypedMetricBuilder) addPeerDeviceSummary(row ddsnmp.BGPRow) {
	b.addAutoDevicePeerCount("configured", 1)
	if row.Admin.Enabled.Has && row.Admin.Enabled.Value {
		b.addAutoDevicePeerCount("admin_enabled", 1)
	}
	if row.State.Has {
		b.addDevicePeerState(string(row.State.State), 1)
		if row.State.State == ddprofiledefinition.BGPPeerStateEstablished {
			b.addAutoDevicePeerCount("established", 1)
		}
	}
}

func (b *bgpTypedMetricBuilder) addPeerLikeRow(pm *ddsnmp.ProfileMetrics, row ddsnmp.BGPRow, scope bgpScope) {
	prefix := buildBGPMetricName(scope, "")
	tags := bgpTypedMetricTags(row, scope)

	availability := make(map[string]int64)
	if row.Admin.Enabled.Has {
		if row.Admin.Enabled.Value {
			availability["admin_enabled"] = 1
		} else {
			availability["admin_enabled"] = 0
			availability["admin_disabled"] = 1
		}
	}
	if row.State.Has {
		if row.State.State == ddprofiledefinition.BGPPeerStateEstablished {
			availability["established"] = 1
		} else {
			availability["established"] = 0
		}
	}
	b.addMetric(pm, prefix+"availability", availability, tags)
	if row.State.Has {
		b.addMetric(pm, prefix+"connection_state", map[string]int64{string(row.State.State): 1}, tags)
	}
	if row.Previous.Has {
		b.addMetric(pm, prefix+"previous_connection_state", map[string]int64{string(row.Previous.State): 1}, tags)
	}
	b.addIntMetric(pm, prefix+"established_uptime", "uptime", row.Connection.EstablishedUptime, tags)
	b.addDirectionalMetric(pm, prefix+"update_traffic", row.Traffic.Updates, tags)
	b.addDirectionalMetric(pm, prefix+"message_traffic", row.Traffic.Messages, tags)
	b.addDirectionalMetric(pm, prefix+"notification_traffic", row.Traffic.Notifications, tags)
	b.addDirectionalMetric(pm, prefix+"route_refresh_traffic", row.Traffic.RouteRefreshes, tags)
	b.addDirectionalMetric(pm, prefix+"open_traffic", row.Traffic.Opens, tags)
	b.addDirectionalMetric(pm, prefix+"keepalive_traffic", row.Traffic.Keepalives, tags)
	b.addIntMetric(pm, prefix+"established_transitions", "transitions", row.Transitions.Established, tags)
	b.addIntMetric(pm, prefix+"down_transitions", "transitions", row.Transitions.Down, tags)
	b.addIntMetric(pm, prefix+"up_transitions", "transitions", row.Transitions.Up, tags)
	b.addIntMetric(pm, prefix+"flaps", "flaps", row.Transitions.Flaps, tags)
	b.addIntMetric(pm, prefix+"last_received_update_age", "age", row.Connection.LastReceivedUpdateAge, tags)
	b.addLastErrorMetric(pm, prefix+"last_error", row.LastError, tags)
	b.addTextStateMetric(pm, prefix+"last_down_reason", row.Reasons.LastDown, tags)
	b.addTextStateMetric(pm, prefix+"last_received_notification_reason", row.LastNotify.Received.Reason, tags)
	b.addTextStateMetric(pm, prefix+"last_sent_notification_reason", row.LastNotify.Sent.Reason, tags)
	b.addTimerPairMetric(pm, prefix+"negotiated_timers", row.Timers.Negotiated, tags)
	b.addTimerPairMetric(pm, prefix+"configured_timers", row.Timers.Configured, tags)

	if scope == bgpScopePeerFamilies {
		b.addTextStateMetric(pm, prefix+"graceful_restart_state", row.Restart.State, tags)
		b.addTextStateMetric(pm, prefix+"unavailability_reason", row.Reasons.Unavailability, tags)
		b.addRouteCountersMetric(pm, prefix+"route_counts.current", row.Routes.Current, tags)
		b.addRouteCountersMetric(pm, prefix+"route_totals", row.Routes.Total, tags)
		b.addRouteLimitsMetric(pm, prefix+"route_limits", row.RouteLimits, tags)
		b.addRouteLimitThresholdsMetric(pm, prefix+"route_limit_thresholds", row.RouteLimits, tags)
	}
}

func (b *bgpTypedMetricBuilder) addMetric(pm *ddsnmp.ProfileMetrics, name string, mv map[string]int64, tags map[string]string) {
	if len(mv) == 0 {
		return
	}
	spec, ok := bgpMetricSpec(name)
	if !ok {
		return
	}
	metric := ddsnmp.Metric{
		Profile:     pm,
		Name:        name,
		Description: spec.description,
		Family:      spec.family,
		Unit:        spec.unit,
		ChartType:   spec.chartType,
		MetricType:  spec.metricType,
		IsTable:     spec.isTable,
		MultiValue:  maps.Clone(mv),
	}
	if len(tags) > 0 {
		metric.Tags = maps.Clone(tags)
	}
	b.metrics = append(b.metrics, metric)
}

func (b *bgpTypedMetricBuilder) addIntMetric(pm *ddsnmp.ProfileMetrics, name, dim string, value ddsnmp.BGPInt64, tags map[string]string) {
	if value.Has {
		b.addMetric(pm, name, map[string]int64{dim: value.Value}, tags)
	}
}

func (b *bgpTypedMetricBuilder) addDirectionalMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPDirectional, tags map[string]string) {
	mv := make(map[string]int64)
	if value.Received.Has {
		mv["received"] = value.Received.Value
	}
	if value.Sent.Has {
		mv["sent"] = value.Sent.Value
	}
	b.addMetric(pm, name, mv, tags)
}

func (b *bgpTypedMetricBuilder) addLastErrorMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPLastError, tags map[string]string) {
	mv := make(map[string]int64)
	if value.Code.Has {
		mv["code"] = value.Code.Value
	}
	if value.Subcode.Has {
		mv["subcode"] = value.Subcode.Value
	}
	b.addMetric(pm, name, mv, tags)
}

func (b *bgpTypedMetricBuilder) addTextStateMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPText, tags map[string]string) {
	if value.Has && value.Value != "" {
		b.addMetric(pm, name, map[string]int64{value.Value: 1}, tags)
	}
}

func (b *bgpTypedMetricBuilder) addTimerPairMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPTimerPair, tags map[string]string) {
	mv := make(map[string]int64)
	if value.ConnectRetry.Has {
		mv["connect_retry"] = value.ConnectRetry.Value
	}
	if value.HoldTime.Has {
		mv["hold_time"] = value.HoldTime.Value
	}
	if value.KeepaliveTime.Has {
		mv["keepalive"] = value.KeepaliveTime.Value
	}
	if value.MinASOriginationInterval.Has {
		mv["min_as_origination_interval"] = value.MinASOriginationInterval.Value
	}
	if value.MinRouteAdvertisementInterval.Has {
		mv["min_route_advertisement_interval"] = value.MinRouteAdvertisementInterval.Value
	}
	b.addMetric(pm, name, mv, tags)
}

func (b *bgpTypedMetricBuilder) addRouteCountersMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPRouteCounters, tags map[string]string) {
	mv := make(map[string]int64)
	if value.Received.Has {
		mv["received"] = value.Received.Value
	}
	if value.Accepted.Has {
		mv["accepted"] = value.Accepted.Value
	}
	if value.Rejected.Has {
		mv["rejected"] = value.Rejected.Value
	}
	if value.Active.Has {
		mv["active"] = value.Active.Value
	}
	if value.Advertised.Has {
		mv["advertised"] = value.Advertised.Value
	}
	if value.Suppressed.Has {
		mv["suppressed"] = value.Suppressed.Value
	}
	if value.Withdrawn.Has {
		mv["withdrawn"] = value.Withdrawn.Value
	}
	b.addMetric(pm, name, mv, tags)
}

func (b *bgpTypedMetricBuilder) addRouteLimitsMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPRouteLimits, tags map[string]string) {
	mv := make(map[string]int64)
	if value.Limit.Has {
		mv["admin_limit"] = value.Limit.Value
	}
	b.addMetric(pm, name, mv, tags)
}

func (b *bgpTypedMetricBuilder) addRouteLimitThresholdsMetric(pm *ddsnmp.ProfileMetrics, name string, value ddsnmp.BGPRouteLimits, tags map[string]string) {
	mv := make(map[string]int64)
	if value.Threshold.Has {
		mv["threshold"] = value.Threshold.Value
	}
	if value.ClearThreshold.Has {
		mv["clear_threshold"] = value.ClearThreshold.Value
	}
	b.addMetric(pm, name, mv, tags)
}

func (b *bgpTypedMetricBuilder) addExplicitDevicePeerCount(dim string, value int64) {
	if b.explicitDevicePeerCounts == nil {
		b.explicitDevicePeerCounts = make(map[string]int64)
	}
	b.explicitDevicePeerCounts[dim] += value
}

func (b *bgpTypedMetricBuilder) addAutoDevicePeerCount(dim string, value int64) {
	if b.autoDevicePeerCounts == nil {
		b.autoDevicePeerCounts = make(map[string]int64)
	}
	b.autoDevicePeerCounts[dim] += value
}

func (b *bgpTypedMetricBuilder) addDevicePeerState(dim string, value int64) {
	if b.devicePeerStates == nil {
		b.devicePeerStates = make(map[string]int64)
	}
	b.devicePeerStates[dim] += value
}

func (b *bgpTypedMetricBuilder) metricsWithDeviceSummaries() []ddsnmp.Metric {
	if b.deviceSample != nil {
		peerCounts := maps.Clone(b.autoDevicePeerCounts)
		if peerCounts == nil && len(b.explicitDevicePeerCounts) > 0 {
			peerCounts = make(map[string]int64, len(b.explicitDevicePeerCounts))
		}
		for dim, value := range b.explicitDevicePeerCounts {
			peerCounts[dim] = value
		}
		b.addMetric(b.deviceSample, "bgp.devices.peer_counts", peerCounts, nil)
		b.addMetric(b.deviceSample, "bgp.devices.peer_states", b.devicePeerStates, nil)
	}
	return b.metrics
}

func bgpTypedMetricTags(row ddsnmp.BGPRow, scope bgpScope) map[string]string {
	tags := make(map[string]string)
	add := func(key, value string) {
		if value != "" {
			tags[key] = value
		}
	}
	add("routing_instance", bgpRoutingInstance(row))
	add("neighbor", firstNonEmpty(row.Identity.Neighbor, row.Tags["neighbor"]))
	add("remote_as", firstNonEmpty(row.Identity.RemoteAS, row.Tags["remote_as"]))
	if scope == bgpScopePeerFamilies {
		add("address_family", string(row.Identity.AddressFamily))
		add("subsequent_address_family", string(row.Identity.SubsequentAddressFamily))
	}
	add("_local_address", row.Descriptors.LocalAddress)
	add("_local_as", row.Descriptors.LocalAS)
	add("_local_identifier", row.Descriptors.LocalIdentifier)
	add("_peer_identifier", row.Descriptors.PeerIdentifier)
	add("_peer_type", row.Descriptors.PeerType)
	add("_bgp_version", row.Descriptors.BGPVersion)
	add("_peer_description", row.Descriptors.Description)
	return tags
}

func bgpRoutingInstance(row ddsnmp.BGPRow) string {
	return firstNonEmpty(row.Identity.RoutingInstance, row.Tags["routing_instance"], "default")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
