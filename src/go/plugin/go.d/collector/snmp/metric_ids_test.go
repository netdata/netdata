// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
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
		"table dimension identity preserves its complete subkey": {
			call: func() string { return metricIDFromKey("snmp_device_prof_value_digest", "rx.bytes") },
			want: "snmp_device_prof_value_digest_dim_19d114af95b3f52858fb79c334c6c76e43e14da258a332343e8293fcd7bfa60e",
		},
		"canonical table tuple has a stable encoding": {
			call: func() string {
				return tableMetricKey(ddsnmp.Metric{Name: "value", Tags: map[string]string{"port": "1"}})
			},
			want: "snmp_device_prof_value_8543248f1ffbe6b474635119197afbd983caa4e085aa446bd59fe7720c3a0aae",
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
