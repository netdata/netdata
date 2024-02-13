// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "github.com/netdata/go.d.plugin/pkg/prometheus"

const (
	metricTCPConnectionFailure               = "windows_tcp_connection_failures_total"
	metricTCPConnectionActive                = "windows_tcp_connections_active_total"
	metricTCPConnectionEstablished           = "windows_tcp_connections_established"
	metricTCPConnectionPassive               = "windows_tcp_connections_passive_total"
	metricTCPConnectionReset                 = "windows_tcp_connections_reset_total"
	metricTCPConnectionSegmentsReceived      = "windows_tcp_segments_received_total"
	metricTCPConnectionSegmentsRetransmitted = "windows_tcp_segments_retransmitted_total"
	metricTCPConnectionSegmentsSent          = "windows_tcp_segments_sent_total"
)

func (w *Windows) collectTCP(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorTCP] {
		w.cache.collection[collectorTCP] = true
		w.addTCPCharts()
	}

	px := "tcp_"
	for _, pm := range pms.FindByName(metricTCPConnectionFailure) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_conns_failures"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionActive) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_conns_active"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionEstablished) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_conns_established"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionPassive) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_conns_passive"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionReset) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_conns_resets"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionSegmentsReceived) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_segments_received"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionSegmentsRetransmitted) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_segments_retransmitted"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricTCPConnectionSegmentsSent) {
		if af := pm.Labels.Get("af"); af != "" {
			mx[px+af+"_segments_sent"] = int64(pm.Value)
		}
	}
}
