// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricNetBytesReceivedTotal            = "windows_net_bytes_received_total"
	metricNetBytesSentTotal                = "windows_net_bytes_sent_total"
	metricNetPacketsReceivedTotal          = "windows_net_packets_received_total"
	metricNetPacketsSentTotal              = "windows_net_packets_sent_total"
	metricNetPacketsReceivedDiscardedTotal = "windows_net_packets_received_discarded_total"
	metricNetPacketsOutboundDiscardedTotal = "windows_net_packets_outbound_discarded_total"
	metricNetPacketsReceivedErrorsTotal    = "windows_net_packets_received_errors_total"
	metricNetPacketsOutboundErrorsTotal    = "windows_net_packets_outbound_errors_total"
)

func (w *Windows) collectNet(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "net_nic_"
	for _, pm := range pms.FindByName(metricNetBytesReceivedTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_bytes_received"] += int64(pm.Value * 8)
		}
	}
	for _, pm := range pms.FindByName(metricNetBytesSentTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_bytes_sent"] += int64(pm.Value * 8)
		}
	}
	for _, pm := range pms.FindByName(metricNetPacketsReceivedTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_packets_received_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricNetPacketsSentTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_packets_sent_total"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricNetPacketsReceivedDiscardedTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_packets_received_discarded"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricNetPacketsOutboundDiscardedTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_packets_outbound_discarded"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricNetPacketsReceivedErrorsTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_packets_received_errors"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricNetPacketsOutboundErrorsTotal) {
		if nic := cleanNICID(pm.Labels.Get("nic")); nic != "" {
			seen[nic] = true
			mx[px+nic+"_packets_outbound_errors"] += int64(pm.Value)
		}
	}

	for nic := range seen {
		if !w.cache.nics[nic] {
			w.cache.nics[nic] = true
			w.addNICCharts(nic)
		}
	}
	for nic := range w.cache.nics {
		if !seen[nic] {
			delete(w.cache.nics, nic)
			w.removeNICCharts(nic)
		}
	}
}

func cleanNICID(id string) string {
	return strings.Replace(id, "__", "_", -1)
}
