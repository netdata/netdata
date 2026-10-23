// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"testing"
)

// Scope-refresh cache measurements (2026-10-24, linux/amd64, i9-12900K).
// Six alternating before/after runs, 300ms per benchmark, compared with benchstat:
// vec_10_series: 7899 -> 8255 ns/op; vec_100_series: 70790 -> 72700 ns/op
// (neither cycle-time difference significant). Allocation counts remain 76/622.
// Scoped lookup adds metadata comparison: 272.5 -> 319 ns/op (+17%, p=0.002),
// unchanged 376 B / 5 allocations. The 100-series cycle varies by 0.01% B/op.
// Command: go test ./pkg/metrix -run '^$' -bench 'VecScopedLookup|VecVsWithLabelsSnapshotGauge' -benchmem
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
