// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "testing"

func TestMetricIDFromName_GenericProfilesKeepLegacyRawIDs(t *testing.T) {
	got := metricIDFromName("metric.with space", "sub.key with space")
	want := "snmp_device_prof_metric.with space_sub.key with space"
	if got != want {
		t.Fatalf("metricIDFromName() = %q, want %q", got, want)
	}
}

func TestMetricIDFromKey_GenericProfilesKeepLegacyRawIDs(t *testing.T) {
	got := metricIDFromKey("metric_192.0.2.10 with space", "rx.bytes")
	want := "snmp_device_prof_metric_192.0.2.10 with space_rx.bytes"
	if got != want {
		t.Fatalf("metricIDFromKey() = %q, want %q", got, want)
	}
}

func TestChartIDFromKey_GenericProfilesStayCleaned(t *testing.T) {
	got := chartIDFromKey("metric_192.0.2.10 with space")
	want := "snmp_device_prof_metric_192_0_2_10_with_space"
	if got != want {
		t.Fatalf("chartIDFromKey() = %q, want %q", got, want)
	}
}

func TestMetricIDFromName_BGPPublicMetricsUseCleanedIDs(t *testing.T) {
	got := metricIDFromName("bgp.peers.message_traffic", "received.total")
	want := "snmp_bgp_peers_message_traffic_received_total"
	if got != want {
		t.Fatalf("metricIDFromName() = %q, want %q", got, want)
	}
}
