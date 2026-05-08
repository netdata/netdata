// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricIDs(t *testing.T) {
	tests := map[string]struct {
		call func() string
		want string
	}{
		"generic profile metric name keeps legacy raw ID": {
			call: func() string { return metricIDFromName("metric.with space", "sub.key with space") },
			want: "snmp_device_prof_metric.with space_sub.key with space",
		},
		"generic profile metric key keeps legacy raw ID": {
			call: func() string { return metricIDFromKey("metric_192.0.2.10 with space", "rx.bytes") },
			want: "snmp_device_prof_metric_192.0.2.10 with space_rx.bytes",
		},
		"generic profile chart key stays cleaned": {
			call: func() string { return chartIDFromKey("metric_192.0.2.10 with space") },
			want: "snmp_device_prof_metric_192_0_2_10_with_space",
		},
		"BGP chart metric name uses cleaned ID": {
			call: func() string { return metricIDFromName("bgp.peers.message_traffic", "received.total") },
			want: "snmp_bgp_peers_message_traffic_received_total",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.call())
		})
	}
}
