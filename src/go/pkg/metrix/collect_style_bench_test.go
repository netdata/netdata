// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"testing"
)

// Baseline results (2026-02-13, darwin/arm64, Apple M4 Pro):
// Command:
//
//	go test ./pkg/metrix -run '^$' -bench 'CollectStyleSnapshot' -benchmem
//
// BenchmarkCollectStyleSnapshot/lazy_declare_3_metrics-14    764806   1544 ns/op   3912 B/op   57 allocs/op
// BenchmarkCollectStyleSnapshot/cached_handles_3_metrics-14  940884   1286 ns/op   3208 B/op   44 allocs/op
// BenchmarkCollectStyleSnapshot/lazy_declare_20_metrics-14   138122   8734 ns/op  20792 B/op  291 allocs/op
// BenchmarkCollectStyleSnapshot/cached_handles_20_metrics-14 167974   7197 ns/op  16960 B/op  227 allocs/op
//
// Keep this block updated when changing metrix write/declaration paths.
func BenchmarkCollectStyleSnapshot(b *testing.B) {
	tests := map[string]struct {
		totalMetrics int
		lazyDeclare  bool
	}{
		"lazy_declare_3_metrics": {
			totalMetrics: 3,
			lazyDeclare:  true,
		},
		"cached_handles_3_metrics": {
			totalMetrics: 3,
			lazyDeclare:  false,
		},
		"lazy_declare_20_metrics": {
			totalMetrics: 20,
			lazyDeclare:  true,
		},
		"cached_handles_20_metrics": {
			totalMetrics: 20,
			lazyDeclare:  false,
		},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			s := NewCollectorStore()
			cc := benchmarkCycleController(b, s)
			base := s.Write().SnapshotMeter("bench.collect")
			instanceLS := base.LabelSet(Label{Key: "instance", Value: "collector-A"})

			gaugeNames, counterNames := benchmarkMetricNames(tc.totalMetrics)
			var cachedGauges []SnapshotGauge
			var cachedCounters []SnapshotCounter

			if !tc.lazyDeclare {
				sm := s.Write().SnapshotMeter("bench.collect").WithLabelSet(instanceLS)
				cachedGauges = make([]SnapshotGauge, 0, len(gaugeNames))
				cachedCounters = make([]SnapshotCounter, 0, len(counterNames))
				for _, n := range gaugeNames {
					cachedGauges = append(cachedGauges, sm.Gauge(n))
				}
				for _, n := range counterNames {
					cachedCounters = append(cachedCounters, sm.Counter(n))
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cc.BeginCycle()
				if tc.lazyDeclare {
					sm := s.Write().SnapshotMeter("bench.collect").WithLabelSet(instanceLS)
					for j, n := range gaugeNames {
						sm.Gauge(n).Observe(SampleValue(j + i))
					}
					for j, n := range counterNames {
						sm.Counter(n).ObserveTotal(SampleValue(1000 + j + i))
					}
				} else {
					for j := range cachedGauges {
						cachedGauges[j].Observe(SampleValue(j + i))
					}
					for j := range cachedCounters {
						cachedCounters[j].ObserveTotal(SampleValue(1000 + j + i))
					}
				}
				cc.CommitCycleSuccess()
			}
		})
	}
}

func benchmarkMetricNames(total int) (gauges []string, counters []string) {
	if total < 1 {
		return []string{"gauge0"}, nil
	}
	gaugeCount := total / 2
	counterCount := total - gaugeCount
	if gaugeCount == 0 {
		gaugeCount = 1
		counterCount = total - gaugeCount
	}
	if counterCount == 0 {
		counterCount = 1
		gaugeCount = total - counterCount
	}

	gauges = make([]string, 0, gaugeCount)
	counters = make([]string, 0, counterCount)
	for i := 0; i < gaugeCount; i++ {
		gauges = append(gauges, "gauge_"+strconv.Itoa(i))
	}
	for i := 0; i < counterCount; i++ {
		counters = append(counters, "counter_"+strconv.Itoa(i))
	}
	return gauges, counters
}
