// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenSnapshotScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"malformed histogram cumulative length is skipped safely": {
			run: func(t *testing.T) {
				src := &readSnapshot{
					series: map[string]*committedSeries{
						"svc.latency": {
							key:                 "svc.latency",
							name:                "svc.latency",
							desc:                &instrumentDescriptor{name: "svc.latency", kind: kindHistogram, mode: modeSnapshot, freshness: FreshnessCycle, histogram: &histogramSchema{bounds: []float64{1, 2}}},
							meta:                SeriesMeta{LastSeenSuccessSeq: 1},
							value:               0,
							histogramCount:      2,
							histogramSum:        3,
							histogramCumulative: []SampleValue{1}, // malformed: len 1, bounds len 2
						},
					},
				}

				flat := flattenSnapshot(src)
				r := &storeReader{snap: flat}

				_, ok := r.Value("svc.latency_bucket", Labels{"le": "1"})
				require.False(t, ok, "expected malformed histogram bucket series to be skipped")
				_, ok = r.Value("svc.latency_count", nil)
				require.False(t, ok, "expected malformed histogram count series to be skipped")
				_, ok = r.Value("svc.latency_sum", nil)
				require.False(t, ok, "expected malformed histogram sum series to be skipped")
			},
		},
		"malformed summary quantile length skips whole flattened family": {
			run: func(t *testing.T) {
				src := &readSnapshot{
					series: map[string]*committedSeries{
						"svc.latency": {
							key:   "svc.latency",
							name:  "svc.latency",
							desc:  &instrumentDescriptor{name: "svc.latency", kind: kindSummary, mode: modeSnapshot, freshness: FreshnessCycle, summary: &summarySchema{quantiles: []float64{0.5, 0.9}}},
							meta:  SeriesMeta{LastSeenSuccessSeq: 1},
							value: 0,

							summaryCount:     2,
							summarySum:       1.2,
							summaryQuantiles: []SampleValue{0.4}, // malformed: len 1, quantiles len 2
						},
					},
				}

				flat := flattenSnapshot(src)
				r := &storeReader{snap: flat, raw: true}

				_, ok := r.Value("svc.latency_count", nil)
				require.False(t, ok, "expected malformed summary count series to be skipped")
				_, ok = r.Value("svc.latency_sum", nil)
				require.False(t, ok, "expected malformed summary sum series to be skipped")
				_, ok = r.Value("svc.latency", Labels{"quantile": "0.5"})
				require.False(t, ok, "expected malformed summary quantile series to be skipped")
			},
		},
		"malformed summary quantiles without configured quantiles skips whole flattened family": {
			run: func(t *testing.T) {
				src := &readSnapshot{
					series: map[string]*committedSeries{
						"svc.latency": {
							key:   "svc.latency",
							name:  "svc.latency",
							desc:  &instrumentDescriptor{name: "svc.latency", kind: kindSummary, mode: modeSnapshot, freshness: FreshnessCycle, summary: &summarySchema{}},
							meta:  SeriesMeta{LastSeenSuccessSeq: 1},
							value: 0,

							summaryCount:     2,
							summarySum:       1.2,
							summaryQuantiles: []SampleValue{0.4}, // malformed: no quantiles configured
						},
					},
				}

				flat := flattenSnapshot(src)
				r := &storeReader{snap: flat, raw: true}

				_, ok := r.Value("svc.latency_count", nil)
				require.False(t, ok, "expected malformed summary count series to be skipped")
				_, ok = r.Value("svc.latency_sum", nil)
				require.False(t, ok, "expected malformed summary sum series to be skipped")
				_, ok = r.Value("svc.latency", Labels{"quantile": "0.5"})
				require.False(t, ok, "expected malformed summary quantile series to be skipped")
			},
		},
		"flattened histogram and summary counters support delta": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				m := s.Write().StatefulMeter("svc")
				h := m.Histogram("latency", WithHistogramBounds(1, 2))
				sum := m.Summary("duration")

				cc.BeginCycle()
				h.Observe(0.5)
				h.Observe(1.5)
				sum.Observe(2)
				sum.Observe(4)
				require.NoError(t, cc.CommitCycleSuccess())

				mustNoDelta(t, s.Read(ReadFlatten()), "svc.latency_bucket", Labels{HistogramBucketLabel: "1"})
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.latency_count", nil)
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.latency_sum", nil)
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.duration_count", nil)
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.duration_sum", nil)

				cc.BeginCycle()
				h.Observe(0.2)
				h.Observe(3)
				sum.Observe(6)
				require.NoError(t, cc.CommitCycleSuccess())

				fr := s.Read(ReadFlatten())
				mustDelta(t, fr, "svc.latency_bucket", Labels{HistogramBucketLabel: "1"}, 1)
				mustDelta(t, fr, "svc.latency_bucket", Labels{HistogramBucketLabel: "2"}, 0)
				mustDelta(t, fr, "svc.latency_bucket", Labels{HistogramBucketLabel: "+Inf"}, 1)
				mustDelta(t, fr, "svc.latency_count", nil, 2)
				mustDelta(t, fr, "svc.latency_sum", nil, 3.2)
				mustDelta(t, fr, "svc.duration_count", nil, 1)
				mustDelta(t, fr, "svc.duration_sum", nil, 6)
			},
		},
		"flattened histogram and summary sum deltas are unavailable on decrease": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				m := s.Write().StatefulMeter("svc")
				h := m.Histogram("latency", WithHistogramBounds(0, 10))
				sum := m.Summary("duration")

				cc.BeginCycle()
				h.Observe(10)
				sum.Observe(10)
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				h.Observe(-5)
				sum.Observe(-5)
				require.NoError(t, cc.CommitCycleSuccess())

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "svc.latency_sum", nil, 5)
				mustValue(t, fr, "svc.duration_sum", nil, 5)
				mustNoDelta(t, fr, "svc.latency_sum", nil)
				mustNoDelta(t, fr, "svc.duration_sum", nil)
				mustDelta(t, fr, "svc.latency_count", nil, 1)
				mustDelta(t, fr, "svc.duration_count", nil, 1)
			},
		},
		"flattened histogram and summary counters require contiguous successful cycles": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				m := s.Write().StatefulMeter("svc")
				h := m.Histogram("latency", WithHistogramBounds(1, 2))
				sum := m.Summary("duration")

				cc.BeginCycle()
				h.Observe(1)
				sum.Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				h.Observe(2)
				sum.Observe(2)
				require.NoError(t, cc.CommitCycleSuccess())

				fr := s.Read(ReadFlatten())
				mustNoDelta(t, fr, "svc.latency_bucket", Labels{HistogramBucketLabel: "2"})
				mustNoDelta(t, fr, "svc.latency_count", nil)
				mustNoDelta(t, fr, "svc.latency_sum", nil)
				mustNoDelta(t, fr, "svc.duration_count", nil)
				mustNoDelta(t, fr, "svc.duration_sum", nil)
			},
		},
		"snapshot histogram and summary reset semantics are preserved when flattened": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				m := s.Write().SnapshotMeter("svc")
				h := m.Histogram("latency", WithHistogramBounds(1, 2))
				sum := m.Summary("duration")

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 10,
					Sum:   10,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 8},
						{UpperBound: 2, CumulativeCount: 10},
					},
				})
				sum.ObservePoint(SummaryPoint{Count: 10, Sum: 10})
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				h.ObservePoint(HistogramPoint{
					Count: 2,
					Sum:   5,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 2},
					},
				})
				sum.ObservePoint(SummaryPoint{Count: 2, Sum: 5})
				require.NoError(t, cc.CommitCycleSuccess())

				fr := s.Read(ReadFlatten())
				mustDelta(t, fr, "svc.latency_bucket", Labels{HistogramBucketLabel: "1"}, 1)
				mustDelta(t, fr, "svc.latency_bucket", Labels{HistogramBucketLabel: "2"}, 1)
				mustDelta(t, fr, "svc.latency_count", nil, 2)
				mustNoDelta(t, fr, "svc.latency_sum", nil)
				mustDelta(t, fr, "svc.duration_count", nil, 2)
				mustNoDelta(t, fr, "svc.duration_sum", nil)
			},
		},
		"flattened reader remains immutable after later commits": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				m := s.Write().StatefulMeter("svc")
				h := m.Histogram("latency", WithHistogramBounds(1, 2))
				sum := m.Summary("duration")

				cc.BeginCycle()
				h.Observe(0.5)
				sum.Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				h.Observe(1.5)
				sum.Observe(2)
				require.NoError(t, cc.CommitCycleSuccess())

				oldReader := s.Read(ReadFlatten())
				mustValue(t, oldReader, "svc.latency_count", nil, 2)
				mustDelta(t, oldReader, "svc.latency_bucket", Labels{HistogramBucketLabel: "2"}, 1)
				mustDelta(t, oldReader, "svc.duration_sum", nil, 2)

				cc.BeginCycle()
				h.Observe(3)
				sum.Observe(4)
				require.NoError(t, cc.CommitCycleSuccess())

				mustValue(t, oldReader, "svc.latency_count", nil, 2)
				mustDelta(t, oldReader, "svc.latency_bucket", Labels{HistogramBucketLabel: "2"}, 1)
				mustDelta(t, oldReader, "svc.duration_sum", nil, 2)

				newReader := s.Read(ReadFlatten())
				mustValue(t, newReader, "svc.latency_count", nil, 3)
				mustDelta(t, newReader, "svc.latency_bucket", Labels{HistogramBucketLabel: "+Inf"}, 1)
				mustDelta(t, newReader, "svc.duration_sum", nil, 4)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
