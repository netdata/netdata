// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

// Baseline results (2026-02-13, darwin/arm64, Apple M4 Pro):
// Command:
//
//	go test ./pkg/metrix -run '^$' -bench 'RuntimeStore' -benchmem
//
// BenchmarkRuntimeStoreCounterParallelAdd/p1-14             2407790     495.2 ns/op   1040 B/op     9 allocs/op
// BenchmarkRuntimeStoreCounterParallelAdd/p4-14             2488057     503.1 ns/op   1040 B/op     9 allocs/op
// BenchmarkRuntimeStoreCounterParallelAdd/p16-14            2054728     580.1 ns/op   1040 B/op     9 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p1-14               2453990     487.7 ns/op   1040 B/op     9 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p4-14               2452767     492.4 ns/op   1040 B/op     9 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p16-14              1964413     591.8 ns/op   1040 B/op     9 allocs/op
// BenchmarkRuntimeStoreMixedTypedWriteAndReadFlatten-14       60756   18938   ns/op  71350 B/op   237 allocs/op
//
// After runtime lazy reader index optimization (2026-02-13):
//
// BenchmarkRuntimeStoreCounterParallelAdd/p1-14             3417770     344.1 ns/op    560 B/op     4 allocs/op
// BenchmarkRuntimeStoreCounterParallelAdd/p4-14             3289084     358.2 ns/op    560 B/op     4 allocs/op
// BenchmarkRuntimeStoreCounterParallelAdd/p16-14            2758773     434.2 ns/op    560 B/op     4 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p1-14               3440482     347.4 ns/op    560 B/op     4 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p4-14               3268348     372.4 ns/op    560 B/op     4 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p16-14              2722263     436.0 ns/op    560 B/op     4 allocs/op
// BenchmarkRuntimeStoreMixedTypedWriteAndReadFlatten-14       97585   10916   ns/op  13366 B/op   155 allocs/op
//
// After runtime overlay+compaction write path (2026-02-19):
//
// BenchmarkRuntimeStoreCounterParallelAdd/p1-14             3759434     308.4 ns/op    622 B/op     4 allocs/op
// BenchmarkRuntimeStoreCounterParallelAdd/p4-14             3838068     312.3 ns/op    622 B/op     4 allocs/op
// BenchmarkRuntimeStoreCounterParallelAdd/p16-14            3553308     374.8 ns/op    622 B/op     4 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p1-14               3852787     309.8 ns/op    622 B/op     4 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p4-14               3797656     314.0 ns/op    622 B/op     4 allocs/op
// BenchmarkRuntimeStoreGaugeParallelSet/p16-14              3520359     341.7 ns/op    622 B/op     4 allocs/op
// BenchmarkRuntimeStoreMixedTypedWriteAndReadFlatten-14       93160   12230   ns/op  15275 B/op   159 allocs/op
//
// After structured flatten allocation optimization (2026-07-27):
// Merge-base production beede2920e18ec0e0efa3c522367ce26deec1281 ->
// optimized production 1f59fa84ba593a0560663d1ce185542485eddf7d.
// The 759296896a6758c2d8919df169a3d14807d6650d version of this benchmark
// was overlaid unchanged onto clean trees at both revisions. Results are
// medians of -count=10 on the same developer laptop. ns/op is a trend
// indicator, while bytes and allocations describe the stable structural
// improvement.
//
//	go test -run '^$' -bench '^BenchmarkRuntimeStoreMixedTypedWriteAndReadFlatten$' -benchmem -benchtime=100ms -count=10 ./pkg/metrix
//
// BenchmarkRuntimeStoreMixedTypedWriteAndReadFlatten:
// ns/op -4.4% local trend; 20,943 -> 20,103 B/op; 157 -> 148 allocs/op.
func BenchmarkRuntimeStoreCounterParallelAdd(b *testing.B) {
	tests := map[string]struct {
		parallelism int
	}{
		"p1":  {parallelism: 1},
		"p4":  {parallelism: 4},
		"p16": {parallelism: 16},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewRuntimeStore()
			c := s.Write().StatefulMeter("runtime").Counter("events_total")

			b.SetParallelism(tc.parallelism)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					c.Add(1)
				}
			})
		})
	}
}

func BenchmarkRuntimeStoreGaugeParallelSet(b *testing.B) {
	tests := map[string]struct {
		parallelism int
	}{
		"p1":  {parallelism: 1},
		"p4":  {parallelism: 4},
		"p16": {parallelism: 16},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewRuntimeStore()
			g := s.Write().StatefulMeter("runtime").Gauge("heap_bytes")

			b.SetParallelism(tc.parallelism)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var v SampleValue
				for pb.Next() {
					g.Set(v)
					v += 1
				}
			})
		})
	}
}

func BenchmarkRuntimeStoreMixedTypedWriteAndReadFlatten(b *testing.B) {
	s := NewRuntimeStore()
	m := s.Write().StatefulMeter("runtime")
	g := m.Gauge("queue_depth")
	c := m.Counter("jobs_total")
	h := m.Histogram("latency", WithHistogramBounds(1, 2, 5))
	sum := m.Summary("request_time", WithSummaryQuantiles(0.5, 0.9, 0.99))
	ss := m.StateSet("mode", WithStateSetStates("maintenance", "operational"), WithStateSetMode(ModeEnum))

	// Seed stateful instruments so the preflight matches timed read visibility.
	g.Set(0)
	c.Add(1)
	h.Observe(1)
	sum.Observe(1)
	ss.Enable("operational")

	preflight := s.Read(ReadFlatten())
	count := 0
	preflight.ForEachSeries(func(_ string, _ LabelView, _ SampleValue) {
		count++
	})
	if count != 15 {
		b.Fatalf("expected 15 flattened series, got %d", count)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Set(SampleValue(i % 100))
		c.Add(1)
		h.Observe(SampleValue((i % 7) + 1))
		sum.Observe(SampleValue((i % 11) + 1))
		ss.Enable("operational")

		// Simulate chart-engine scalar path over runtime metrics.
		r := s.Read(ReadFlatten())
		r.ForEachSeries(func(_ string, _ LabelView, _ SampleValue) {})
	}
}
