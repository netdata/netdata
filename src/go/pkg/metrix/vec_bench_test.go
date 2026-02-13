// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"testing"
)

// Baseline results (2026-02-13, darwin/arm64, Apple M4 Pro):
// Command:
//
//	go test ./pkg/metrix -run '^$' -bench 'VecVsWithLabelsSnapshotGauge' -benchmem
//
// BenchmarkVecVsWithLabelsSnapshotGauge/vec_10_series-14          201846   6021 ns/op   11536 B/op    205 allocs/op
// BenchmarkVecVsWithLabelsSnapshotGauge/with_labels_100_series-14  17137  66114 ns/op  127057 B/op   2134 allocs/op
// BenchmarkVecVsWithLabelsSnapshotGauge/vec_100_series-14           19335  62846 ns/op  106258 B/op   1834 allocs/op
// BenchmarkVecVsWithLabelsSnapshotGauge/with_labels_10_series-14   182618   6575 ns/op   13616 B/op    235 allocs/op
//
// Keep this block updated when changing vec or label-merging write paths.
func BenchmarkVecVsWithLabelsSnapshotGauge(b *testing.B) {
	tests := map[string]struct {
		seriesCount int
		useVec      bool
	}{
		"with_labels_10_series": {
			seriesCount: 10,
			useVec:      false,
		},
		"vec_10_series": {
			seriesCount: 10,
			useVec:      true,
		},
		"with_labels_100_series": {
			seriesCount: 100,
			useVec:      false,
		},
		"vec_100_series": {
			seriesCount: 100,
			useVec:      true,
		},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			states := benchmarkVecStates(tc.seriesCount)
			if tc.useVec {
				vec := s.Write().SnapshotMeter("bench.vec").GaugeVec("workers", []string{"instance", "state"})
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cc.BeginCycle()
					for _, state := range states {
						vec.WithLabelValues("job-a", state).Observe(SampleValue(i))
					}
					cc.CommitCycleSuccess()
				}
				return
			}

			sm := s.Write().SnapshotMeter("bench.vec")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cc.BeginCycle()
				for _, state := range states {
					sm.WithLabels(
						Label{Key: "instance", Value: "job-a"},
						Label{Key: "state", Value: state},
					).Gauge("workers").Observe(SampleValue(i))
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

func benchmarkVecStates(n int) []string {
	if n < 1 {
		n = 1
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = "s" + strconv.Itoa(i)
	}
	return out
}
