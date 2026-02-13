// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"testing"
)

func TestSummaryStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot summary count sum read and flatten": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)
				sm := s.Write().SnapshotMeter("svc")
				sum := sm.Summary("latency")

				cc.BeginCycle()
				sum.ObservePoint(SummaryPoint{Count: 3, Sum: 1.2})
				cc.CommitCycleSuccess()

				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{Count: 3, Sum: 1.2})
				if _, ok := s.Read().Value("svc.latency", nil); ok {
					t.Fatalf("expected non-scalar summary unavailable via Value")
				}

				fr := s.Read().Flatten()
				mustValue(t, fr, "svc.latency_count", nil, 3)
				mustValue(t, fr, "svc.latency_sum", nil, 1.2)
				if _, ok := fr.Summary("svc.latency", nil); ok {
					t.Fatalf("expected flattened reader to hide typed summary getter")
				}
			},
		},
		"snapshot summary quantiles are validated and flattened": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)
				sum := s.Write().SnapshotMeter("svc").Summary("latency", WithSummaryQuantiles(0.5, 0.9))

				cc.BeginCycle()
				sum.ObservePoint(SummaryPoint{
					Count: 2,
					Sum:   1.0,
					Quantiles: []QuantilePoint{
						{Quantile: 0.9, Value: 0.8},
						{Quantile: 0.5, Value: 0.4},
					},
				})
				cc.CommitCycleSuccess()

				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{
					Count: 2,
					Sum:   1.0,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 0.4},
						{Quantile: 0.9, Value: 0.8},
					},
				})

				fr := s.Read().Flatten()
				mustValue(t, fr, "svc.latency", Labels{"quantile": "0.5"}, 0.4)
				mustValue(t, fr, "svc.latency", Labels{"quantile": "0.9"}, 0.8)
				mustValue(t, fr, "svc.latency_count", nil, 2)
				mustValue(t, fr, "svc.latency_sum", nil, 1.0)
			},
		},
		"snapshot summary point quantiles must match declaration": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)
				sum := s.Write().SnapshotMeter("svc").Summary("latency", WithSummaryQuantiles(0.5, 0.9))

				cc.BeginCycle()
				expectPanic(t, func() {
					sum.ObservePoint(SummaryPoint{
						Count: 1,
						Sum:   0.2,
						Quantiles: []QuantilePoint{
							{Quantile: 0.5, Value: 0.2},
						},
					})
				})
				cc.AbortCycle()
			},
		},
		"stateful summary cumulative accumulates count sum and quantiles": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)
				sum := s.Write().StatefulMeter("svc").Summary("latency", WithSummaryQuantiles(0.5, 1.0))

				cc.BeginCycle()
				sum.Observe(1)
				sum.Observe(2)
				sum.Observe(10)
				cc.CommitCycleSuccess()
				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{
					Count: 3,
					Sum:   13,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 2},
						{Quantile: 1.0, Value: 10},
					},
				})

				cc.BeginCycle()
				sum.Observe(3)
				cc.CommitCycleSuccess()
				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{
					Count: 4,
					Sum:   16,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 2.5},
						{Quantile: 1.0, Value: 10},
					},
				})
			},
		},
		"stateful summary cycle window resets and uses freshness cycle": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)
				sum := s.Write().StatefulMeter("svc").Summary(
					"latency",
					WithSummaryQuantiles(0.5),
					WithWindow(WindowCycle),
				)

				cc.BeginCycle()
				sum.Observe(2)
				sum.Observe(4)
				cc.CommitCycleSuccess()
				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{
					Count: 2,
					Sum:   6,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 3},
					},
				})

				cc.BeginCycle()
				sum.Observe(1)
				cc.CommitCycleSuccess()
				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{
					Count: 1,
					Sum:   1,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 1},
					},
				})

				cc.BeginCycle()
				cc.CommitCycleSuccess()
				if _, ok := s.Read().Summary("svc.latency", nil); ok {
					t.Fatalf("expected stale cycle-window summary hidden from Read")
				}
				mustSummary(t, s.ReadRaw(), "svc.latency", nil, SummaryPoint{
					Count: 1,
					Sum:   1,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 1},
					},
				})
				if _, ok := s.Read().Flatten().Value("svc.latency_count", nil); ok {
					t.Fatalf("expected stale cycle-window summary flatten hidden from Read().Flatten()")
				}
				mustValue(t, s.ReadRaw().Flatten(), "svc.latency_count", nil, 1)
			},
		},
		"window option on snapshot summary panics": {
			run: func(t *testing.T) {
				s := NewStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Summary("latency", WithWindow(WindowCycle))
				})
			},
		},
		"summary reservoir option validation and custom size": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)

				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Summary("latency", WithSummaryReservoirSize(128))
				})
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("svc").Summary("latency_bad", WithSummaryQuantiles(0.9), WithSummaryReservoirSize(0))
				})

				sum := s.Write().StatefulMeter("svc").Summary(
					"latency_small_reservoir",
					WithSummaryQuantiles(0.5),
					WithSummaryReservoirSize(8),
				)
				cc.BeginCycle()
				for i := 0; i < 100; i++ {
					sum.Observe(SampleValue(i))
				}
				cc.CommitCycleSuccess()

				p, ok := s.Read().Summary("svc.latency_small_reservoir", nil)
				if !ok {
					t.Fatalf("expected summary for custom reservoir series")
				}
				if p.Count != 100 {
					t.Fatalf("unexpected count: got=%v want=100", p.Count)
				}
				if len(p.Quantiles) != 1 || math.IsNaN(p.Quantiles[0].Value) {
					t.Fatalf("expected non-NaN quantile value with custom reservoir")
				}
			},
		},
		"summary flatten quantile label collision panics": {
			run: func(t *testing.T) {
				s := NewStore()
				cc := cycleController(t, s)
				sum := s.Write().SnapshotMeter("svc").
					WithLabels(Label{Key: "quantile", Value: "x"}).
					Summary("latency", WithSummaryQuantiles(0.5))

				cc.BeginCycle()
				expectPanic(t, func() {
					sum.ObservePoint(SummaryPoint{
						Count: 1,
						Sum:   0.2,
						Quantiles: []QuantilePoint{
							{Quantile: 0.5, Value: 0.2},
						},
					})
				})
				cc.AbortCycle()
			},
		},
		"summary schema mismatch and mode mixing panic": {
			run: func(t *testing.T) {
				s := NewStore()
				_ = s.Write().SnapshotMeter("svc").Summary("latency", WithSummaryQuantiles(0.5))

				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Summary("latency", WithSummaryQuantiles(0.9))
				})
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("svc").Summary("latency", WithSummaryQuantiles(0.5))
				})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func mustSummary(t *testing.T, r Reader, name string, labels Labels, want SummaryPoint) {
	t.Helper()
	got, ok := r.Summary(name, labels)
	if !ok {
		t.Fatalf("expected summary for %s", name)
	}
	if !equalSample(got.Count, want.Count) || !equalSample(got.Sum, want.Sum) {
		t.Fatalf("unexpected summary count/sum for %s: got=(%v,%v) want=(%v,%v)", name, got.Count, got.Sum, want.Count, want.Sum)
	}
	if len(got.Quantiles) != len(want.Quantiles) {
		t.Fatalf("unexpected summary quantiles length for %s: got=%d want=%d", name, len(got.Quantiles), len(want.Quantiles))
	}
	for i := range want.Quantiles {
		gq := got.Quantiles[i]
		wq := want.Quantiles[i]
		if gq.Quantile != wq.Quantile || !equalSample(gq.Value, wq.Value) {
			t.Fatalf("unexpected summary quantile[%d] for %s: got=(%v,%v) want=(%v,%v)", i, name, gq.Quantile, gq.Value, wq.Quantile, wq.Value)
		}
	}
}

func equalSample(a, b SampleValue) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	const eps = 1e-9
	return math.Abs(a-b) <= eps
}
