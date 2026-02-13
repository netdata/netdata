// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

func BenchmarkSummaryStatefulWindowCycle(b *testing.B) {
	tests := map[string]struct {
		samples   int
		quantiles []float64
	}{
		"100_samples_3q": {
			samples:   100,
			quantiles: []float64{0.5, 0.9, 0.99},
		},
		"1000_samples_3q": {
			samples:   1000,
			quantiles: []float64{0.5, 0.9, 0.99},
		},
		"5000_samples_3q": {
			samples:   5000,
			quantiles: []float64{0.5, 0.9, 0.99},
		},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			sum := s.Write().StatefulMeter("bench").Summary(
				"latency_seconds",
				WithSummaryQuantiles(tc.quantiles...),
				WithWindow(WindowCycle),
			)
			values := benchmarkSummaryValues(tc.samples)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cc.BeginCycle()
				for _, v := range values {
					sum.Observe(v)
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

func BenchmarkSummaryStatefulCumulativeWithPreload(b *testing.B) {
	tests := map[string]struct {
		preload   int
		add       int
		quantiles []float64
	}{
		"preload_1000_add_100_3q": {
			preload:   1000,
			add:       100,
			quantiles: []float64{0.5, 0.9, 0.99},
		},
		"preload_10000_add_100_3q": {
			preload:   10000,
			add:       100,
			quantiles: []float64{0.5, 0.9, 0.99},
		},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			preloadValues := benchmarkSummaryValues(tc.preload)
			addValues := benchmarkSummaryValues(tc.add)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				s := NewCollectorStore()
				cc := benchmarkCycleController(b, s)
				sum := s.Write().StatefulMeter("bench").Summary(
					"latency_seconds",
					WithSummaryQuantiles(tc.quantiles...),
					WithWindow(WindowCumulative),
				)
				cc.BeginCycle()
				for _, v := range preloadValues {
					sum.Observe(v)
				}
				cc.CommitCycleSuccess()
				b.StartTimer()

				cc.BeginCycle()
				for _, v := range addValues {
					sum.Observe(v)
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

func BenchmarkSummarySnapshotObservePoint(b *testing.B) {
	tests := map[string]struct {
		quantiles []float64
	}{
		"count_sum_only": {
			quantiles: nil,
		},
		"with_3q": {
			quantiles: []float64{0.5, 0.9, 0.99},
		},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			sum := s.Write().SnapshotMeter("bench").Summary("latency_seconds", WithSummaryQuantiles(tc.quantiles...))

			point := SummaryPoint{
				Count: 100,
				Sum:   12.34,
			}
			if len(tc.quantiles) > 0 {
				point.Quantiles = []QuantilePoint{
					{Quantile: 0.5, Value: 0.12},
					{Quantile: 0.9, Value: 0.88},
					{Quantile: 0.99, Value: 1.2},
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cc.BeginCycle()
				sum.ObservePoint(point)
				cc.CommitCycleSuccess()
			}
		})
	}
}

func benchmarkCycleController(b *testing.B, s CollectorStore) CycleController {
	b.Helper()
	managed, ok := AsCycleManagedStore(s)
	if !ok {
		b.Fatalf("store does not expose cycle control")
	}
	return managed.CycleController()
}

func benchmarkSummaryValues(n int) []SampleValue {
	vals := make([]SampleValue, n)
	var x uint64 = 0x9e3779b97f4a7c15
	for i := 0; i < n; i++ {
		// Deterministic pseudo-random in [0,1).
		x ^= x >> 12
		x ^= x << 25
		x ^= x >> 27
		y := x * 2685821657736338717
		vals[i] = SampleValue(float64(y&0xffffffff) / float64(1<<32))
	}
	return vals
}
