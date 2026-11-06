// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// Distinct tag maps represent consecutive polls. Unchanged
// labels should not allocate per-chart snapshots or scan the chart collection.
func BenchmarkTableChartLabels(b *testing.B) {
	for _, rows := range []int{32, 4096} {
		for _, changed := range []bool{false, true} {
			b.Run(fmt.Sprintf("rows=%d/changed=%t", rows, changed), func(b *testing.B) {
				c := newTestSNMPCollector()
				c.Hostname = "192.0.2.1"
				c.sysInfo = &snmputils.SysInfo{
					Name: "switch",
				}
				var batches [2][]ddsnmp.Metric
				for frame := range batches {
					for i := range rows {
						alias := "endpoint-a"
						if changed && frame == 1 {
							alias = "endpoint-b"
						}
						batches[frame] = append(batches[frame], ddsnmp.Metric{
							Name:    "port_power",
							IsTable: true,
							Value:   7,
							Tags: map[string]string{
								"port":       fmt.Sprint(i),
								"_interface": fmt.Sprintf("Gi1/0/%d", i),
								"_if_alias":  alias,
							},
							Profile: &ddsnmp.ProfileMetrics{},
						})
					}
				}
				samples := make(map[string]int64)
				c.collectProfileTableMetrics(samples, batches[0])
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; b.Loop(); n++ {
					clear(samples)
					c.collectProfileTableMetrics(samples, batches[n%2])
				}
			})
		}
	}
}
