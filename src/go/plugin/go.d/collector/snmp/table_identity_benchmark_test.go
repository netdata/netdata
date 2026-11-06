// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

var tableIdentityBenchmarkKey string

func BenchmarkTableIdentity(b *testing.B) {
	for name, metric := range map[string]ddsnmp.Metric{
		"interface": {Name: "ifHCInOctets", Tags: map[string]string{"interface": "Ethernet1", "_if_alias": "uplink"}},
		"compound":  {Name: "jnxFWCounterPacketCount", Tags: map[string]string{"filter": "filter_a.b", "counter": "allow_2_c", "counter_type": "2"}},
		"long":      {Name: "jnxFWCounterPacketCount", Tags: map[string]string{"filter": strings.Repeat("f", 127), "counter": strings.Repeat("c", 127), "counter_type": "2"}},
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tableIdentityBenchmarkKey = tableMetricKey(metric)
			}
		})
	}
}

// Exercise statesets/counter groups as well as single-valued table rows.
func BenchmarkTableMultiValueIdentity(b *testing.B) {
	for _, count := range []int{32, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			c := newTestSNMPCollector()
			c.sysInfo = &snmputils.SysInfo{}
			rows := make([]ddsnmp.Metric, count)
			for i := range rows {
				rows[i] = ddsnmp.Metric{
					Name:    "bgp.peers.state",
					IsTable: true,
					Profile: &ddsnmp.ProfileMetrics{},
					Tags: map[string]string{
						"routing_instance": "default",
						"neighbor":         "2001:db8::" + strconv.FormatInt(int64(i+1), 16),
						"remote_as":        "64512",
					},
					MultiValue: map[string]int64{
						"idle":        0,
						"connect":     0,
						"active":      0,
						"opensent":    0,
						"openconfirm": 0,
						"established": 1,
					},
				}
			}
			samples := map[string]int64{}
			c.collectProfileTableMetrics(samples, rows)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				clear(samples)
				c.collectProfileTableMetrics(samples, rows)
			}
		})
	}
}
