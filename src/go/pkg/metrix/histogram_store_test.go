// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistogramStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot histogram with explicit bounds read and flatten": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram("request_duration_seconds", WithHistogramBounds(0.1, 0.5, 1))

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 3,
					Sum:   1.2,
					Buckets: []BucketPoint{
						{UpperBound: 0.1, CumulativeCount: 1},
						{UpperBound: 0.5, CumulativeCount: 2},
						{UpperBound: 1, CumulativeCount: 3},
					},
				})
				cc.CommitCycleSuccess()

				mustHistogram(t, s.Read(), "svc.request_duration_seconds", nil, HistogramPoint{
					Count: 3,
					Sum:   1.2,
					Buckets: []BucketPoint{
						{UpperBound: 0.1, CumulativeCount: 1},
						{UpperBound: 0.5, CumulativeCount: 2},
						{UpperBound: 1, CumulativeCount: 3},
					},
				})
				_, ok := s.Read().Value("svc.request_duration_seconds", nil)
				require.False(t, ok, "expected non-scalar histogram unavailable via Value")

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "svc.request_duration_seconds_bucket", Labels{"le": "0.1"}, 1)
				mustValue(t, fr, "svc.request_duration_seconds_bucket", Labels{"le": "0.5"}, 2)
				mustValue(t, fr, "svc.request_duration_seconds_bucket", Labels{"le": "1"}, 3)
				mustValue(t, fr, "svc.request_duration_seconds_bucket", Labels{"le": "+Inf"}, 3)
				mustValue(t, fr, "svc.request_duration_seconds_count", nil, 3)
				mustValue(t, fr, "svc.request_duration_seconds_sum", nil, 1.2)
			},
		},
		"snapshot histogram without bounds captures schema after successful cycle": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram("latency")
				view, ok := s.(*storeView)
				require.True(t, ok, "expected *storeView")

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 1, Sum: 0.2,
					Buckets: []BucketPoint{
						{UpperBound: 0.5, CumulativeCount: 1},
						{UpperBound: 1, CumulativeCount: 1},
					},
				})
				cc.AbortCycle() // schema must not be captured on abort

				desc := view.core.instruments["svc.latency"]
				require.NotNil(t, desc, "expected registered descriptor")
				require.Nil(t, desc.histogram, "expected shared descriptor histogram schema to remain unset after abort")

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 2, Sum: 0.8,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 2},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})
				cc.CommitCycleSuccess()

				// Shared descriptor remains immutable after publish; per-series descriptor carries schema.
				require.Nil(t, desc.histogram, "expected shared descriptor histogram schema to remain unset")
				mustHistogram(t, s.Read(), "svc.latency", nil, HistogramPoint{
					Count: 2, Sum: 0.8,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 2},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})

				cc.BeginCycle()
				expectPanic(t, func() {
					h.ObservePoint(HistogramPoint{
						Count: 2, Sum: 0.8,
						Buckets: []BucketPoint{
							{UpperBound: 0.5, CumulativeCount: 2},
							{UpperBound: 1, CumulativeCount: 2},
						},
					})
				})
				cc.AbortCycle()
			},
		},
		"stateful histogram requires bounds": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("svc").Histogram("latency")
				})
			},
		},
		"stateful histogram cumulative window accumulates across cycles": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().StatefulMeter("svc").Histogram("latency", WithHistogramBounds(1, 2))

				cc.BeginCycle()
				h.Observe(0.5)
				h.Observe(1.5)
				h.Observe(3)
				cc.CommitCycleSuccess()
				mustHistogram(t, s.Read(), "svc.latency", nil, HistogramPoint{
					Count: 3, Sum: 5,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})

				cc.BeginCycle()
				h.Observe(0.2)
				cc.CommitCycleSuccess()
				mustHistogram(t, s.Read(), "svc.latency", nil, HistogramPoint{
					Count: 4, Sum: 5.2,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 2},
						{UpperBound: 2, CumulativeCount: 3},
					},
				})
			},
		},
		"stateful histogram window cycle resets each cycle and uses FreshnessCycle": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().StatefulMeter("svc").Histogram("latency", WithHistogramBounds(1, 2), WithWindow(WindowCycle))

				cc.BeginCycle()
				h.Observe(0.5)
				h.Observe(1.5)
				cc.CommitCycleSuccess()
				mustHistogram(t, s.Read(), "svc.latency", nil, HistogramPoint{
					Count: 2, Sum: 2,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})

				cc.BeginCycle()
				h.Observe(1.7)
				cc.CommitCycleSuccess()
				mustHistogram(t, s.Read(), "svc.latency", nil, HistogramPoint{
					Count: 1, Sum: 1.7,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 0},
						{UpperBound: 2, CumulativeCount: 1},
					},
				})

				cc.BeginCycle()
				cc.CommitCycleSuccess()
				_, ok := s.Read().Histogram("svc.latency", nil)
				require.False(t, ok, "expected stale cycle-window histogram hidden from Read")
				_, ok = s.Read(ReadRaw()).Histogram("svc.latency", nil)
				require.True(t, ok, "expected raw histogram to remain visible")
			},
		},
		"window option on snapshot histogram panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Histogram("latency", WithWindow(WindowCycle))
				})
			},
		},
		"histogram point validation panics on invalid buckets": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram("latency")

				cc.BeginCycle()
				expectPanic(t, func() {
					h.ObservePoint(HistogramPoint{
						Count: 2, Sum: 1.0,
						Buckets: []BucketPoint{
							{UpperBound: 1, CumulativeCount: 2},
							{UpperBound: 0.5, CumulativeCount: 2},
						},
					})
				})
				cc.AbortCycle()

				cc.BeginCycle()
				expectPanic(t, func() {
					h.ObservePoint(HistogramPoint{
						Count: 2, Sum: 1.0,
						Buckets: []BucketPoint{
							{UpperBound: 0.5, CumulativeCount: 2},
							{UpperBound: 1, CumulativeCount: 1},
						},
					})
				})
				cc.AbortCycle()
			},
		},
		"snapshot histogram stale visibility behavior for flatten": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().SnapshotMeter("svc").Histogram("latency", WithHistogramBounds(1))

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 1, Sum: 0.1,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
					},
				})
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				cc.CommitCycleSuccess()

				_, ok := s.Read(ReadFlatten()).Value("svc.latency_bucket", Labels{"le": "1"})
				require.False(t, ok, "expected stale snapshot flattened histogram hidden from Read(ReadFlatten())")
				mustValue(t, s.Read(ReadRaw(), ReadFlatten()), "svc.latency_bucket", Labels{"le": "1"}, 1)
			},
		},
		"histogram flatten label collision panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				h := s.Write().SnapshotMeter("svc").
					WithLabels(Label{Key: "le", Value: "x"}).
					Histogram("latency", WithHistogramBounds(1))

				cc.BeginCycle()
				expectPanic(t, func() {
					h.ObservePoint(HistogramPoint{
						Count: 1, Sum: 0.1,
						Buckets: []BucketPoint{
							{UpperBound: 1, CumulativeCount: 1},
						},
					})
				})
				cc.AbortCycle()
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func mustHistogram(t *testing.T, r Reader, name string, labels Labels, want HistogramPoint) {
	t.Helper()
	got, ok := r.Histogram(name, labels)
	require.True(t, ok, "expected histogram for %s", name)
	require.Equal(t, want.Count, got.Count, "unexpected histogram count for %s", name)
	require.Equal(t, want.Sum, got.Sum, "unexpected histogram sum for %s", name)
	require.Len(t, got.Buckets, len(want.Buckets), "unexpected histogram bucket count for %s", name)
	for i := range want.Buckets {
		gb := got.Buckets[i]
		wb := want.Buckets[i]
		require.Equal(t, wb.UpperBound, gb.UpperBound, "unexpected histogram bucket upper-bound[%d] for %s", i, name)
		require.Equal(t, wb.CumulativeCount, gb.CumulativeCount, "unexpected histogram bucket cumulative count[%d] for %s", i, name)
	}
}
