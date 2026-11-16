// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"testing"
)

// Pointer-only scope-cache measurements (2026-10-24, darwin/arm64, Apple M4 Pro, Go 1.27.3).
// Six alternating 300ms runs, comparing copied cache scopes with instrument accessors using benchstat:
// vec_10_series: 6316 -> 6512 ns/op; vec_100_series: 58670 -> 58670 ns/op.
// with_labels_10_series: 9540 -> 9575 ns/op; with_labels_100_series: 89610 -> 89880 ns/op.
// No cycle-time difference is significant. Vec allocation counts remain 76/622;
// with_labels allocation counts remain 166/1522.
// Scoped lookup: 226.5 -> 225.8 ns/op (not significant), unchanged 376 B / 5 allocations.
// Cache population, 100/1000 series: 51.00/644.6 -> 37.05/396.9 KiB/op, unchanged 931/9042 allocations.
// Population time: 28.86/324.6 -> 26.61/291.5 us/op. Timings are local trends, not CI gates.
// Command: go test ./pkg/metrix -run '^$' -bench 'VecScopedLookup|VecVsWithLabelsSnapshotGauge|VecCachePopulate' -benchmem
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
				vec := s.Write().SnapshotMeter("bench.vec").Vec("instance", "state").Gauge("workers")
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

// BenchmarkVecCachePopulate includes the storage cost hidden by warm-cache lookup benchmarks.
func BenchmarkVecCachePopulate(b *testing.B) {
	for _, seriesCount := range []int{100, 1000} {
		b.Run(strconv.Itoa(seriesCount)+"_series", func(b *testing.B) {
			values := benchmarkVecStates(seriesCount)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				vec := NewCollectorStore().Write().SnapshotMeter("bench.vec").Vec("instance").Gauge("workers")
				for _, value := range values {
					vec.WithLabelValues(value)
				}
			}
		})
	}
}
