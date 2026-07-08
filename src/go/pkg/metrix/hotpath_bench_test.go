// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"testing"
)

// Hot-path benchmarks for the collector store commit/record path. Every number in this
// file - this header's history and the "Latest" comment above each benchmark - was
// measured on a developer laptop, NOT CI. Treat them as relative/trend indicators for
// before/after comparisons, not absolute gates.
//
// Baseline results (historical; each benchmark's latest numbers are inline above its func):
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
// Latest (developer laptop, -benchtime=200x): series_1 625ns/5allocs, series_10 990ns/5,
// series_100 1.0us/5, series_1000 2.0us/5. Per-write cost is ~flat across cardinality.
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

// This guards the ALLOCATION envelope of a sparse commit: only touched/changed series are
// cloned, so allocs stay ~O(touched) and never O(retained). Commit TIME is intentionally
// O(live-series + touched + distinct-authorities) - every commit copies the series map and
// scans live series for retention, host-scope refresh, canonicalization, and the
// descriptor-universe sweep - so the timings scale with live series (same touched, more
// retained = slower), which is expected; the invariant this bench pins is that allocs do not.
// Latest (developer laptop, -benchtime=300x): s100/t5 2.2us/38allocs, s1000/t5 11us/38,
// s5000/t10 23us/58, s50000/t10 198us/62, s100000/t20 543us/98 (the descriptor-universe sweep
// added no allocs: it is O(distinct-names) and its live-name set does not escape the commit).
// Before the canonicalization-clone fix the s100000/t20 case cloned every retained descriptor:
// 43.7ms / 100340 allocs.
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

// BenchmarkCollectorCommitManySuperseded exercises commit when many distinct names each
// change kind (supersede) in one cycle. Must be O(live), not O(superseded*live).
// Round-8 set-based supersede, before -> after (developer laptop): names_100 304us -> 91us;
// names_1000 4.87ms -> 1.10ms; names_5000 91.1ms -> 5.68ms. O(N^2) -> O(N).
func BenchmarkCollectorCommitManySuperseded(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("names_%d", n), func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			b.ReportAllocs()
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				// Alternate each name's kind every cycle so each name supersedes the other.
				cc.BeginCycle()
				meter := s.Write().SnapshotMeter("svc")
				for i := 0; i < n; i++ {
					name := fmt.Sprintf("m%d", i)
					if iter%2 == 0 {
						meter.Counter(name).ObserveTotal(1)
					} else {
						meter.Gauge(name).Observe(1)
					}
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

// BenchmarkCollectorCommitManyDropped exercises commit when many distinct names are
// ambiguous (two incompatible kinds, no committed authority) and dropped. Must be
// O(touched), not O(dropped*touched).
// Round-8 set-based drop, before -> after (developer laptop): names_100 177us -> 96us;
// names_1000 8.58ms -> 1.15ms; names_5000 196ms -> 6.03ms. O(N^2) -> O(N).
func BenchmarkCollectorCommitManyDropped(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("names_%d", n), func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			b.ReportAllocs()
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				cc.BeginCycle()
				meter := s.Write().SnapshotMeter("svc")
				for i := 0; i < n; i++ {
					name := fmt.Sprintf("m%d", i)
					meter.Gauge(name).Observe(1)
					meter.Counter(name).ObserveTotal(1)
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

// BenchmarkCollectorSameKeyRepeatedWrites exercises repeated same-key writes whose observed
// schema differs from the staged entry but is otherwise identical (the same conflicting bounds
// every time). They share one authority fingerprint, so the conflict is recorded once and every
// later repeat is dropped by a no-clone identity check - both the descriptor evidence AND the
// effective-descriptor clones stay O(1), not O(samples) (the residual per-sample cost is inherent
// point normalization).
// Fingerprint dedup with no-clone repeat check, before -> after (developer laptop): samples_10000
// 1.81ms / 100045 allocs -> 0.97ms / 20044 allocs (the residual allocs are inherent point
// normalization; the per-sample effective clone and evidence growth are gone once the conflicting
// authority is recorded). This is the pathological conflict-flood path; a healthy same-schema
// repeat never enters here.
func BenchmarkCollectorSameKeyRepeatedWrites(b *testing.B) {
	pt := func(uppers ...float64) HistogramPoint {
		buckets := make([]BucketPoint, len(uppers))
		for i, ub := range uppers {
			buckets[i] = BucketPoint{UpperBound: ub, CumulativeCount: 1}
		}
		return HistogramPoint{Count: 1, Sum: 1, Buckets: buckets}
	}
	for _, k := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("samples_%d", k), func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			pt12, pt56 := pt(1, 2), pt(5, 6)
			b.ReportAllocs()
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				cc.BeginCycle()
				h := s.Write().SnapshotMeter("svc").Histogram("h")
				h.ObservePoint(pt12)
				for j := 0; j < k; j++ {
					h.ObservePoint(pt56) // differs from the staged entry every time
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

// BenchmarkCollectorCommitManySchemasPerName exercises one histogram name whose labels
// carry many distinct bucket schemas (distinct authorities). Authority grouping is
// fingerprint-indexed, so it is O(distinct) time, not O(distinct^2).
// History (developer laptop): pre-hardening O(N^2) (schemas_1000 29.8ms / 1.52M allocs); a
// cap-at-2 made it O(N) time / O(1) memory but LOSSILY hid declaration conflicts past the cap; the
// fingerprint index restores losslessness at O(distinct) memory - schemas_1000 ~688us / ~25k allocs
// (every distinct authority is kept so any declaration conflict is caught). This is the pathological
// many-schema-per-name case (a broken collector); a healthy 1-authority name is unaffected.
func BenchmarkCollectorCommitManySchemasPerName(b *testing.B) {
	for _, n := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("schemas_%d", n), func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			b.ReportAllocs()
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				cc.BeginCycle()
				meter := s.Write().SnapshotMeter("svc")
				for i := 0; i < n; i++ {
					pt := HistogramPoint{Count: 1, Sum: 1, Buckets: []BucketPoint{{UpperBound: float64(i + 1), CumulativeCount: 1}}}
					meter.WithLabels(Label{Key: "id", Value: fmt.Sprintf("s%d", i)}).Histogram("h").ObservePoint(pt)
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

// BenchmarkCollectorSameKeyManyDeclarations exercises one series key written by many handles that
// share an authority but declare DISTINCT metadata (unique units). Conflict evidence is keyed by
// full descriptor identity (authority fingerprint + declaration fingerprint), so recording each
// distinct declaration is O(1) and the total stays O(distinct), not O(distinct^2) (a per-authority
// bucket rescanned before every append).
// Full-descriptor conflict key, before -> after (developer laptop): declarations_1000 2.76ms ->
// 0.39ms (allocs stay O(N): 6.1k -> 7.1k). The win is TIME: the per-append bucket scan (O(N^2)) is
// replaced by an O(1) keyed lookup.
func BenchmarkCollectorSameKeyManyDeclarations(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("declarations_%d", n), func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			// N stale same-key gauge handles, same authority, distinct units (each from its own
			// aborted cycle so registration does not reject the declaration conflict).
			handles := make([]SnapshotGauge, n)
			for i := range handles {
				cc.BeginCycle()
				handles[i] = s.Write().SnapshotMeter("svc").Gauge("m", WithUnit(fmt.Sprintf("u%d", i)))
				cc.AbortCycle()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				cc.BeginCycle()
				for _, h := range handles {
					h.Observe(1)
				}
				cc.CommitCycleSuccess() // fails on the declaration conflicts; measures conflict recording + resolution
			}
		})
	}
}
