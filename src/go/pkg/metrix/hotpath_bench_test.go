// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"testing"
)

// Baseline results (to be updated before/after hot-path refactors):
// Command:
//
//	go test ./pkg/metrix -run '^$' -bench 'RuntimeStoreSingleWriteAtCardinality|CollectorCommitSparseAtCardinality' -benchmem
//
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_1-14             3574240      318.1 ns/op      696 B/op   8 allocs/op
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_10-14            2242100      536.9 ns/op      944 B/op  10 allocs/op
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_100-14            499005       2421 ns/op     3984 B/op  10 allocs/op
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_1000-14            46905      25925 ns/op    55096 B/op  12 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_100_touched_5-14   710890       1664 ns/op     3864 B/op  49 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_1000_touched_5-14  628440       1741 ns/op     3870 B/op  49 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_5000_touched_10-14 279890       4242 ns/op     7205 B/op  85 allocs/op
//
// After runtime overlay+collector clone-on-modify changes (2026-02-19):
//
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_1-14             4073148      294.3 ns/op      726 B/op   8 allocs/op
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_10-14            3756022      297.6 ns/op      729 B/op   8 allocs/op
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_100-14           3635368      331.2 ns/op      777 B/op   8 allocs/op
// BenchmarkRuntimeStoreSingleWriteAtCardinality/series_1000-14          1550416      730.2 ns/op     1575 B/op   8 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_100_touched_5-14    667111      1787 ns/op     3880 B/op  49 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_1000_touched_5-14   615114      1858 ns/op     3881 B/op  49 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_5000_touched_10-14  240637      4481 ns/op     7166 B/op  85 allocs/op
//
// After collector work-elimination slice (1A + 2B + 3B) (2026-02-19):
//
// BenchmarkCollectorCommitSparseAtCardinality/series_50000_touched_10-14  978680      2471 ns/op     5697 B/op  45 allocs/op
// BenchmarkCollectorCommitSparseAtCardinality/series_100000_touched_20-14 511872      4346 ns/op    11316 B/op  77 allocs/op
func BenchmarkRuntimeStoreSingleWriteAtCardinality(b *testing.B) {
	tests := map[string]struct {
		totalSeries int
	}{
		"series_1":    {totalSeries: 1},
		"series_10":   {totalSeries: 10},
		"series_100":  {totalSeries: 100},
		"series_1000": {totalSeries: 1000},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewRuntimeStore()
			gv := s.Write().StatefulMeter("runtime.hotpath").Vec("id").Gauge("value")

			for i := 0; i < tc.totalSeries; i++ {
				h, err := gv.GetWithLabelValues(fmt.Sprintf("s%d", i))
				if err != nil {
					b.Fatalf("prepopulate handle: %v", err)
				}
				h.Set(SampleValue(i))
			}

			hot, err := gv.GetWithLabelValues("s0")
			if err != nil {
				b.Fatalf("hot handle: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				hot.Set(SampleValue(i))
			}
		})
	}
}

func BenchmarkCollectorCommitSparseAtCardinality(b *testing.B) {
	tests := map[string]struct {
		totalSeries int
		touched     int
	}{
		"series_100_touched_5":     {totalSeries: 100, touched: 5},
		"series_1000_touched_5":    {totalSeries: 1000, touched: 5},
		"series_5000_touched_10":   {totalSeries: 5000, touched: 10},
		"series_50000_touched_10":  {totalSeries: 50000, touched: 10},
		"series_100000_touched_20": {totalSeries: 100000, touched: 20},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			gv := s.Write().SnapshotMeter("collect.hotpath").Vec("id").Gauge("value")

			all := make([]SnapshotGauge, 0, tc.totalSeries)
			for i := 0; i < tc.totalSeries; i++ {
				h, err := gv.GetWithLabelValues(fmt.Sprintf("s%d", i))
				if err != nil {
					b.Fatalf("prepopulate handle: %v", err)
				}
				all = append(all, h)
			}

			// Create committed baseline cardinality.
			cc.BeginCycle()
			for i, h := range all {
				h.Observe(SampleValue(i))
			}
			cc.CommitCycleSuccess()

			hot := all[:tc.touched]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cc.BeginCycle()
				for j, h := range hot {
					h.Observe(SampleValue(i + j))
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}
