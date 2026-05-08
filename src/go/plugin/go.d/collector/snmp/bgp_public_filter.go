// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func filterChartMetrics(metrics []ddsnmp.Metric) []ddsnmp.Metric {
	if len(metrics) == 0 {
		return nil
	}

	filtered := make([]ddsnmp.Metric, 0, len(metrics))
	for _, metric := range metrics {
		if shouldHideBGPDiagnosticMetric(metric.Name) {
			continue
		}
		filtered = append(filtered, metric)
	}
	return filtered
}

func shouldHideBGPDiagnosticMetric(name string) bool {
	switch name {
	case "bgp.peers.previous_connection_state",
		"bgp.peers.last_error",
		"bgp.peers.last_down_reason",
		"bgp.peers.last_received_notification_reason",
		"bgp.peers.last_sent_notification_reason",
		"bgp.peer_families.last_error",
		"bgp.peer_families.graceful_restart_state",
		"bgp.peer_families.unavailability_reason":
		return true
	default:
		return false
	}
}

func isBGPPeerFunctionMetric(name string) bool {
	return strings.HasPrefix(name, "bgp.peers.") || strings.HasPrefix(name, "bgp.peer_families.")
}
