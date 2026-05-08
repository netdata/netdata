// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func bgpPublicSpec(name string) (bgpPublicMetricSpec, bool) {
	switch name {
	case "bgp.devices.peer_counts":
		return bgpPublicMetricSpec{
			description: "BGP device peer counts",
			family:      "BGP/Devices",
			unit:        "peers",
			metricType:  ddprofiledefinition.ProfileMetricTypeGauge,
		}, true
	case "bgp.devices.peer_states":
		return bgpPublicMetricSpec{
			description: "BGP device peer states",
			family:      "BGP/Devices",
			unit:        "peers",
			metricType:  ddprofiledefinition.ProfileMetricTypeGauge,
		}, true
	case "bgp.peers.availability":
		return tableSpec("BGP peer availability", "BGP/Peers", "status", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.connection_state":
		return tableSpec("BGP peer connection state", "BGP/Peers", "status", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.previous_connection_state":
		return tableSpec("BGP peer previous connection state", "BGP/Peers", "status", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.established_uptime":
		return tableSpec("BGP peer established uptime", "BGP/Peers", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.update_traffic":
		return tableSpec("BGP peer update traffic", "BGP/Peers/Traffic", "updates/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.message_traffic":
		return tableSpec("BGP peer message traffic", "BGP/Peers/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.notification_traffic":
		return tableSpec("BGP peer notification traffic", "BGP/Peers/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.route_refresh_traffic":
		return tableSpec("BGP peer route refresh traffic", "BGP/Peers/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.open_traffic":
		return tableSpec("BGP peer open traffic", "BGP/Peers/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.keepalive_traffic":
		return tableSpec("BGP peer keepalive traffic", "BGP/Peers/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.established_transitions":
		return tableSpec("BGP peer established transitions", "BGP/Peers/Events", "transitions/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peers.down_transitions":
		return tableSpec("BGP peer down transitions", "BGP/Peers/Events", "transitions", ddprofiledefinition.ProfileMetricTypeMonotonicCount), true
	case "bgp.peers.up_transitions":
		return tableSpec("BGP peer up transitions", "BGP/Peers/Events", "transitions", ddprofiledefinition.ProfileMetricTypeMonotonicCount), true
	case "bgp.peers.flaps":
		return tableSpec("BGP peer flaps", "BGP/Peers/Events", "flaps", ddprofiledefinition.ProfileMetricTypeMonotonicCount), true
	case "bgp.peers.last_received_update_age":
		return tableSpec("BGP peer time since last update", "BGP/Peers", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.last_error":
		return tableSpec("BGP peer last error", "BGP/Peers/Errors", "code", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.last_down_reason":
		return tableSpec("BGP peer last down reason", "BGP/Peers/Errors", "reason", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.last_received_notification_reason":
		return tableSpec("BGP peer last received notification reason", "BGP/Peers/Errors", "reason", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.last_sent_notification_reason":
		return tableSpec("BGP peer last sent notification reason", "BGP/Peers/Errors", "reason", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.negotiated_timers":
		return tableSpec("BGP peer negotiated timers", "BGP/Peers/Timers", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peers.configured_timers":
		return tableSpec("BGP peer configured timers", "BGP/Peers/Timers", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.availability":
		return tableSpec("BGP peer-family availability", "BGP/Peer Families", "status", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.connection_state":
		return tableSpec("BGP peer-family connection state", "BGP/Peer Families", "status", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.established_uptime":
		return tableSpec("BGP peer-family established uptime", "BGP/Peer Families", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.update_traffic":
		return tableSpec("BGP peer-family update traffic", "BGP/Peer Families/Traffic", "updates/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.message_traffic":
		return tableSpec("BGP peer-family message traffic", "BGP/Peer Families/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.notification_traffic":
		return tableSpec("BGP peer-family notification traffic", "BGP/Peer Families/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.route_refresh_traffic":
		return tableSpec("BGP peer-family route refresh traffic", "BGP/Peer Families/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.open_traffic":
		return tableSpec("BGP peer-family open traffic", "BGP/Peer Families/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.keepalive_traffic":
		return tableSpec("BGP peer-family keepalive traffic", "BGP/Peer Families/Traffic", "messages/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.established_transitions":
		return tableSpec("BGP peer-family established transitions", "BGP/Peer Families/Events", "transitions/s", ddprofiledefinition.ProfileMetricTypeRate), true
	case "bgp.peer_families.down_transitions":
		return tableSpec("BGP peer-family down transitions", "BGP/Peer Families/Events", "transitions", ddprofiledefinition.ProfileMetricTypeMonotonicCount), true
	case "bgp.peer_families.last_received_update_age":
		return tableSpec("BGP peer-family time since last update", "BGP/Peer Families", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.last_error":
		return tableSpec("BGP peer-family last error", "BGP/Peer Families/Errors", "code", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.graceful_restart_state":
		return tableSpec("BGP peer-family graceful restart state", "BGP/Peer Families", "status", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.unavailability_reason":
		return tableSpec("BGP peer-family unavailability reason", "BGP/Peer Families/Errors", "reason", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.negotiated_timers":
		return tableSpec("BGP peer-family negotiated timers", "BGP/Peer Families/Timers", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.configured_timers":
		return tableSpec("BGP peer-family configured timers", "BGP/Peer Families/Timers", "seconds", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.route_counts.current":
		return tableSpec("BGP peer-family current route counts", "BGP/Peer Families/Routes", "prefixes", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.route_totals":
		return tableSpec("BGP peer-family route totals", "BGP/Peer Families/Routes", "prefixes", ddprofiledefinition.ProfileMetricTypeMonotonicCount), true
	case "bgp.peer_families.route_limits":
		return tableSpec("BGP peer-family route limits", "BGP/Peer Families/Routes", "prefixes", ddprofiledefinition.ProfileMetricTypeGauge), true
	case "bgp.peer_families.route_limit_thresholds":
		return tableSpec("BGP peer-family route limit thresholds", "BGP/Peer Families/Routes", "%", ddprofiledefinition.ProfileMetricTypeGauge), true
	default:
		return bgpPublicMetricSpec{}, false
	}
}

func tableSpec(description, family, unit string, metricType ddprofiledefinition.ProfileMetricType) bgpPublicMetricSpec {
	return bgpPublicMetricSpec{
		description: description,
		family:      family,
		unit:        unit,
		metricType:  metricType,
		isTable:     true,
	}
}

func mergeMultiValue(dst, src map[string]int64) {
	if len(src) == 0 || dst == nil {
		return
	}
	for key, value := range src {
		dst[key] = value
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
