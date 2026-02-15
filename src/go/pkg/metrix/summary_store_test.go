// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummaryStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot summary count sum read and flatten": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sm := s.Write().SnapshotMeter("svc")
				sum := sm.Summary("latency")

				cc.BeginCycle()
				sum.ObservePoint(SummaryPoint{Count: 3, Sum: 1.2})
				cc.CommitCycleSuccess()

				mustSummary(t, s.Read(), "svc.latency", nil, SummaryPoint{Count: 3, Sum: 1.2})
				_, ok := s.Read().Value("svc.latency", nil)
				require.False(t, ok, "expected non-scalar summary unavailable via Value")

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "svc.latency_count", nil, 3)
				mustValue(t, fr, "svc.latency_sum", nil, 1.2)
				_, ok = fr.Summary("svc.latency", nil)
				require.False(t, ok, "expected flattened reader to hide typed summary getter")
			},
		},
		"snapshot summary quantiles are validated and flattened": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
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

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "svc.latency", Labels{"quantile": "0.5"}, 0.4)
				mustValue(t, fr, "svc.latency", Labels{"quantile": "0.9"}, 0.8)
				mustValue(t, fr, "svc.latency_count", nil, 2)
				mustValue(t, fr, "svc.latency_sum", nil, 1.0)
			},
		},
		"snapshot summary point quantiles must match declaration": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
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
				s := NewCollectorStore()
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
				s := NewCollectorStore()
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
				_, ok := s.Read().Summary("svc.latency", nil)
				require.False(t, ok, "expected stale cycle-window summary hidden from Read")
				mustSummary(t, s.Read(ReadRaw()), "svc.latency", nil, SummaryPoint{
					Count: 1,
					Sum:   1,
					Quantiles: []QuantilePoint{
						{Quantile: 0.5, Value: 1},
					},
				})
				_, ok = s.Read(ReadFlatten()).Value("svc.latency_count", nil)
				require.False(t, ok, "expected stale cycle-window summary flatten hidden from Read(ReadFlatten())")
				mustValue(t, s.Read(ReadRaw(), ReadFlatten()), "svc.latency_count", nil, 1)
			},
		},
		"window option on snapshot summary panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Summary("latency", WithWindow(WindowCycle))
				})
			},
		},
		"summary reservoir option validation and custom size": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
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
				require.True(t, ok, "expected summary for custom reservoir series")
				require.Equal(t, SampleValue(100), p.Count, "unexpected count")
				require.Len(t, p.Quantiles, 1)
				require.False(t, math.IsNaN(p.Quantiles[0].Value), "expected non-NaN quantile value with custom reservoir")
			},
		},
		"summary flatten quantile label collision panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
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
				s := NewCollectorStore()
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
	require.True(t, ok, "expected summary for %s", name)
	require.True(t, equalSample(got.Count, want.Count), "unexpected summary count for %s: got=%v want=%v", name, got.Count, want.Count)
	require.True(t, equalSample(got.Sum, want.Sum), "unexpected summary sum for %s: got=%v want=%v", name, got.Sum, want.Sum)
	require.Len(t, got.Quantiles, len(want.Quantiles), "unexpected summary quantiles length for %s", name)
	for i := range want.Quantiles {
		gq := got.Quantiles[i]
		wq := want.Quantiles[i]
		require.Equal(t, wq.Quantile, gq.Quantile, "unexpected summary quantile[%d] for %s", i, name)
		require.True(t, equalSample(gq.Value, wq.Value), "unexpected summary quantile value[%d] for %s: got=%v want=%v", i, name, gq.Value, wq.Value)
	}
}

func equalSample(a, b SampleValue) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	const eps = 1e-9
	return math.Abs(a-b) <= eps
}
