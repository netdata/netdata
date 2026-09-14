// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func BenchmarkBuildTopologyL3SubnetCandidates(b *testing.B) {
	for _, rowCount := range []int{64, 1024, 4096} {
		b.Run(fmt.Sprintf("rows=%d", rowCount), func(b *testing.B) {
			rows := benchmarkL3SubnetRows(rowCount)
			candidates, stats := buildTopologyL3SubnetCandidates(rows)
			if stats.CandidateSubnets == 0 || len(candidates.Segments) == 0 {
				b.Fatalf("benchmark fixture produced no L3 subnet segments")
			}

			b.ReportAllocs()
			b.ReportMetric(float64(rowCount), "rows/op")
			b.ResetTimer()
			for range b.N {
				candidates, stats := buildTopologyL3SubnetCandidates(rows)
				runtime.KeepAlive(candidates)
				runtime.KeepAlive(stats)
			}
		})
	}
}

func benchmarkL3SubnetRows(rowCount int) []topologymodel.L3Interface {
	rows := make([]topologymodel.L3Interface, rowCount)
	for i := range rows {
		subnet := i / 64
		host := i%64 + 1
		rows[i] = topologymodel.L3Interface{
			DeviceID: fmt.Sprintf("device-%d", i),
			IP:       fmt.Sprintf("198.18.%d.%d", subnet, host),
			Netmask:  "255.255.255.0",
			IfIndex:  fmt.Sprintf("%d", i+1),
		}
	}
	return rows
}
