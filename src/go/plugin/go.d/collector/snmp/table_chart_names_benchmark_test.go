// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// Steady-state table collection with and without rows sharing a chart key (devices reporting the same ifName twice).
// Duplicate rows run the row-ordering comparison on every collection, so their allocs/op MUST match a build without
// alias chart naming. ns/op is a development-machine trend indicator only.
func BenchmarkTableChartCollection(b *testing.B) {
	for name, dups := range map[string]int{"unique": 0, "duplicates": 16} {
		b.Run(name, func(b *testing.B) {
			metrics := tableBenchmarkMetrics(48, dups)
			collr := newTestSNMPCollector()
			collr.sysInfo = &snmputils.SysInfo{Name: "sw"}
			mx := make(map[string]int64)
			collr.collectProfileTableMetrics(mx, metrics)
			collr.collectProfileTableMetrics(mx, metrics)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				collr.collectProfileTableMetrics(mx, metrics)
			}
		})
	}
}

func tableBenchmarkMetrics(ifaces, dups int) []ddsnmp.Metric {
	var metrics []ddsnmp.Metric
	for i := 0; i < ifaces; i++ {
		tags := map[string]string{tagInterface: fmt.Sprintf("eth%d", i), tagIfAlias: "", "_if_type": "ethernetCsmacd"}
		metrics = append(metrics, aliasTestMetric("ifTraffic", tags))
	}
	for i := 0; i < dups; i++ {
		tags := map[string]string{tagInterface: fmt.Sprintf("eth%d", i), tagIfAlias: "", "_if_type": "other"}
		metrics = append(metrics, aliasTestMetric("ifTraffic", tags))
	}
	return metrics
}
