// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"

func routeBGPPublicMetric(name string) (bgpRoute, bool) {
	switch name {
	case "bgpPeerAvailability", "alcatel.bgpPeerAvailability":
		return bgpRoute{leaf: "availability", copyMultiValue: true, scope: bgpScopeAuto}, true
	case "bgpPeerState", "huawei.hwBgpPeerState", "aristaBgp4V2PeerState", "dell.os10bgp4V2PeerState":
		return bgpRoute{leaf: "connection_state", copyMultiValue: true, scope: bgpScopeAuto}, true
	case "bgpPeerPreviousState":
		return bgpRoute{leaf: "previous_connection_state", copyMultiValue: true, scope: bgpScopePeers}, true
	case "bgpPeerFsmEstablishedTime":
		return bgpRoute{leaf: "established_uptime", dim: "uptime", scope: bgpScopeAuto}, true
	case "bgpPeerUpdates", "alcatel.bgpPeerUpdates":
		return bgpRoute{leaf: "update_traffic", copyMultiValue: true, scope: bgpScopeAuto}, true
	case "bgpPeerInTotalMessages", "huawei.hwBgpPeerInTotalMsgCounter", "huawei.hwBgpPeerInTotalMsgs":
		return bgpRoute{leaf: "message_traffic", dim: "received", scope: bgpScopeAuto}, true
	case "bgpPeerOutTotalMessages", "huawei.hwBgpPeerOutTotalMsgCounter", "huawei.hwBgpPeerOutTotalMsgs":
		return bgpRoute{leaf: "message_traffic", dim: "sent", scope: bgpScopeAuto}, true
	case "bgpPeerInNotifications", "alcatel.bgpPeerInNotifications", "huawei.hwBgpPeerInNotificationMsgCounter":
		return bgpRoute{leaf: "notification_traffic", dim: "received", scope: bgpScopeAuto}, true
	case "bgpPeerOutNotifications", "alcatel.bgpPeerOutNotifications", "huawei.hwBgpPeerOutNotificationMsgCounter":
		return bgpRoute{leaf: "notification_traffic", dim: "sent", scope: bgpScopeAuto}, true
	case "alcatel.bgpPeerInRouteRefreshMessages", "huawei.hwBgpPeerInRouteFreshMsgCounter":
		return bgpRoute{leaf: "route_refresh_traffic", dim: "received", scope: bgpScopeAuto}, true
	case "alcatel.bgpPeerOutRouteRefreshMessages", "huawei.hwBgpPeerOutRouteFreshMsgCounter":
		return bgpRoute{leaf: "route_refresh_traffic", dim: "sent", scope: bgpScopeAuto}, true
	case "huawei.hwBgpPeerInOpenMsgCounter":
		return bgpRoute{leaf: "open_traffic", dim: "received", scope: bgpScopeAuto}, true
	case "huawei.hwBgpPeerOutOpenMsgCounter":
		return bgpRoute{leaf: "open_traffic", dim: "sent", scope: bgpScopeAuto}, true
	case "huawei.hwBgpPeerInKeepAliveMsgCounter":
		return bgpRoute{leaf: "keepalive_traffic", dim: "received", scope: bgpScopeAuto}, true
	case "huawei.hwBgpPeerOutKeepAliveMsgCounter":
		return bgpRoute{leaf: "keepalive_traffic", dim: "sent", scope: bgpScopeAuto}, true
	case "bgpPeerFsmEstablishedTransitions", "huawei.hwBgpPeerFsmEstablishedCounter", "huawei.hwBgpPeerFsmEstablishedTransitions":
		return bgpRoute{leaf: "established_transitions", dim: "transitions", scope: bgpScopeAuto}, true
	case "bgpPeerDownTransitions", "huawei.hwBgpPeerDownCounts":
		return bgpRoute{leaf: "down_transitions", dim: "transitions", scope: bgpScopeAuto}, true
	case "bgpPeerUpTransitions":
		return bgpRoute{leaf: "up_transitions", dim: "transitions", scope: bgpScopePeers}, true
	case "bgpPeerFlaps":
		return bgpRoute{leaf: "flaps", dim: "flaps", scope: bgpScopePeers}, true
	case "bgpPeerInUpdateElapsedTime":
		return bgpRoute{leaf: "last_received_update_age", dim: "age", scope: bgpScopeAuto}, true
	case "bgpPeerLastErrorCode", "huawei.hwBgpPeerLastErrorCode":
		return bgpRoute{leaf: "last_error", dim: "code", scope: bgpScopeAuto}, true
	case "bgpPeerLastErrorSubcode", "huawei.hwBgpPeerLastErrorSubcode":
		return bgpRoute{leaf: "last_error", dim: "subcode", scope: bgpScopeAuto}, true
	case "alcatel.bgpPeerLastDownReason":
		return bgpRoute{leaf: "last_down_reason", copyMultiValue: true, scope: bgpScopePeers}, true
	case "alcatel.bgpPeerLastRecvNotifyReason":
		return bgpRoute{leaf: "last_received_notification_reason", copyMultiValue: true, scope: bgpScopePeers}, true
	case "alcatel.bgpPeerLastSentNotifyReason":
		return bgpRoute{leaf: "last_sent_notification_reason", copyMultiValue: true, scope: bgpScopePeers}, true
	case "huawei.hwBgpPeerGRStatus":
		return bgpRoute{leaf: "graceful_restart_state", copyMultiValue: true, scope: bgpScopePeerFamilies}, true
	case "huawei.hwBgpPeerUnAvaiReason":
		return bgpRoute{leaf: "unavailability_reason", copyMultiValue: true, scope: bgpScopePeerFamilies}, true
	case "bgpPeerHoldTime":
		return bgpRoute{leaf: "negotiated_timers", dim: "hold_time", scope: bgpScopeAuto}, true
	case "bgpPeerKeepAlive":
		return bgpRoute{leaf: "negotiated_timers", dim: "keepalive", scope: bgpScopeAuto}, true
	case "bgpPeerConnectRetryInterval":
		return bgpRoute{leaf: "configured_timers", dim: "connect_retry", scope: bgpScopeAuto}, true
	case "bgpPeerHoldTimeConfigured":
		return bgpRoute{leaf: "configured_timers", dim: "hold_time", scope: bgpScopeAuto}, true
	case "bgpPeerKeepAliveConfigured":
		return bgpRoute{leaf: "configured_timers", dim: "keepalive", scope: bgpScopeAuto}, true
	case "bgpPeerMinASOriginationInterval":
		return bgpRoute{leaf: "configured_timers", dim: "min_as_origination_interval", scope: bgpScopeAuto}, true
	case "bgpPeerMinRouteAdvertisementInterval":
		return bgpRoute{leaf: "configured_timers", dim: "min_route_advertisement_interval", scope: bgpScopeAuto}, true
	case "bgpPeerPrefixesReceived":
		return bgpRoute{leaf: "route_counts.current", dim: "received", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesAccepted":
		return bgpRoute{leaf: "route_counts.current", dim: "accepted", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesRejected":
		return bgpRoute{leaf: "route_counts.current", dim: "rejected", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesAdvertised":
		return bgpRoute{leaf: "route_counts.current", dim: "advertised", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesActive":
		return bgpRoute{leaf: "route_counts.current", dim: "active", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesSuppressed":
		return bgpRoute{leaf: "route_counts.current", dim: "suppressed", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesWithdrawn":
		return bgpRoute{leaf: "route_counts.current", dim: "withdrawn", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesReceivedTotal", "huawei.hwBgpPeerPrefixRcvCounter":
		return bgpRoute{leaf: "route_totals", dim: "received", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesAdvertisedTotal", "huawei.hwBgpPeerPrefixAdvCounter":
		return bgpRoute{leaf: "route_totals", dim: "advertised", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixesRejectedTotal":
		return bgpRoute{leaf: "route_totals", dim: "rejected", scope: bgpScopePeerFamilies}, true
	case "huawei.hwBgpPeerPrefixActiveCounter":
		return bgpRoute{leaf: "route_totals", dim: "active", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixAdminLimit":
		return bgpRoute{leaf: "route_limits", dim: "admin_limit", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixThreshold":
		return bgpRoute{leaf: "route_limit_thresholds", dim: "threshold", scope: bgpScopePeerFamilies}, true
	case "bgpPeerPrefixClearThreshold":
		return bgpRoute{leaf: "route_limit_thresholds", dim: "clear_threshold", scope: bgpScopePeerFamilies}, true
	case "huawei.hwBgpPeerSessionNum":
		return bgpRoute{leaf: "peer_counts", dim: "configured", scope: bgpScopeDevices}, true
	case "huawei.hwIBgpPeerSessionNum":
		return bgpRoute{leaf: "peer_counts", dim: "ibgp", scope: bgpScopeDevices}, true
	case "huawei.hwEBgpPeerSessionNum":
		return bgpRoute{leaf: "peer_counts", dim: "ebgp", scope: bgpScopeDevices}, true
	default:
		return bgpRoute{}, false
	}
}

func shouldSuppressBGPRawMetric(name string) bool {
	switch name {
	case "bgpPeerAdminStatus",
		"aristaBgp4V2PeerAdminStatus",
		"dell.os10bgp4V2PeerAdminStatus",
		"huawei.hwBgpPeerAdminStatus",
		"bgpPeerInUpdates",
		"bgpPeerOutUpdates",
		"huawei.hwBgpPeerInUpdateMsgCounter",
		"huawei.hwBgpPeerOutUpdateMsgCounter",
		"huawei.hwBgpPeerInUpdateMsgs",
		"huawei.hwBgpPeerOutUpdateMsgs":
		return true
	default:
		return false
	}
}

func resolveBGPScope(scope bgpScope, tags map[string]string) bgpScope {
	if scope != bgpScopeAuto {
		return scope
	}
	if hasSpecificAddressFamily(tags) {
		return bgpScopePeerFamilies
	}
	return bgpScopePeers
}

func buildBGPPublicMetricName(scope bgpScope, leaf string) string {
	switch scope {
	case bgpScopePeerFamilies:
		return "bgp.peer_families." + leaf
	case bgpScopeDevices:
		return "bgp.devices." + leaf
	default:
		return "bgp.peers." + leaf
	}
}

func hasSpecificAddressFamily(tags map[string]string) bool {
	af := bgpTagValue(tags, "address_family")
	safi := bgpTagValue(tags, "subsequent_address_family")
	return (af != "" && af != "all") || (safi != "" && safi != "all")
}

func publicBGPTags(metric ddsnmp.Metric, scope bgpScope) map[string]string {
	tags := metric.Tags
	if len(tags) == 0 {
		tags = nil
	}

	publicTags := make(map[string]string)
	addIdentityTag := func(key string) {
		if value := bgpTagValue(tags, key); value != "" {
			publicTags[key] = value
		}
	}
	addLabelTag := func(key string) {
		if value := bgpTagValue(tags, key); value != "" {
			publicTags["_"+key] = value
		}
	}

	for _, key := range []string{
		"routing_instance",
		"neighbor",
		"remote_as",
	} {
		addIdentityTag(key)
	}

	for _, key := range []string{
		"bgp_version",
		"neighbor_address_type",
		"local_address",
		"local_address_type",
		"local_as",
		"local_identifier",
		"peer_identifier",
		"peer_description",
		"peer_type",
	} {
		addLabelTag(key)
	}

	if scope == bgpScopePeerFamilies {
		for _, key := range []string{"address_family", "subsequent_address_family"} {
			addIdentityTag(key)
		}
		addLabelTag("address_family_name")
		ensureBGPFamilyTags(publicTags, metric.Table)
	}

	if len(publicTags) == 0 {
		return nil
	}
	return publicTags
}

func ensureBGPFamilyTags(tags map[string]string, table string) {
	if tags["address_family"] == "" {
		switch table {
		case "alaBgpPeerTable":
			tags["address_family"] = "ipv4"
		case "alaBgpPeer6Table":
			tags["address_family"] = "ipv6"
		}
	}
	if tags["subsequent_address_family"] == "" {
		switch table {
		case "alaBgpPeerTable", "alaBgpPeer6Table":
			tags["subsequent_address_family"] = "unicast"
		}
	}
	if tags["_address_family_name"] == "" && tags["address_family"] != "" && tags["subsequent_address_family"] != "" {
		tags["_address_family_name"] = tags["address_family"] + " " + tags["subsequent_address_family"]
	}
}

func bgpTagValue(tags map[string]string, key string) string {
	if value := tags[key]; value != "" {
		return value
	}
	if value := tags["_"+key]; value != "" {
		return value
	}
	return ""
}
