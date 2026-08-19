// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"stateful writes are immediate commit with runtime metadata sequence": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				g := m.Gauge("heap_bytes")
				c := m.Counter("jobs_total")

				g.Set(5)
				mustValue(t, s.Read(), "runtime.heap_bytes", nil, 5)
				meta := s.Read().CollectMeta()
				require.Equal(t, CollectStatusSuccess, meta.LastAttemptStatus, "unexpected metadata after gauge set: %#v", meta)
				require.Equal(t, uint64(1), meta.LastAttemptSeq, "unexpected metadata after gauge set: %#v", meta)
				require.Equal(t, uint64(1), meta.LastSuccessSeq, "unexpected metadata after gauge set: %#v", meta)

				c.Add(10)
				mustValue(t, s.Read(), "runtime.jobs_total", nil, 10)
				mustNoDelta(t, s.Read(), "runtime.jobs_total", nil)
				meta = s.Read().CollectMeta()
				require.Equal(t, uint64(2), meta.LastAttemptSeq, "unexpected metadata after first counter add: %#v", meta)
				require.Equal(t, uint64(2), meta.LastSuccessSeq, "unexpected metadata after first counter add: %#v", meta)

				c.Add(4)
				mustValue(t, s.Read(), "runtime.jobs_total", nil, 14)
				mustDelta(t, s.Read(), "runtime.jobs_total", nil, 4)
				meta = s.Read().CollectMeta()
				require.Equal(t, uint64(3), meta.LastAttemptSeq, "unexpected metadata after second counter add: %#v", meta)
				require.Equal(t, uint64(3), meta.LastSuccessSeq, "unexpected metadata after second counter add: %#v", meta)
			},
		},
		"runtime rejects snapshot-style constraints": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()

				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("runtime").Gauge("x", WithFreshness(FreshnessCycle))
				})
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("runtime").Summary("s", WithSummaryQuantiles(0.5), WithWindow(WindowCycle))
				})
			},
		},
		"runtime summary read and flatten are chart-compatible": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				sum := s.Write().StatefulMeter("runtime").Summary("latency", WithSummaryQuantiles(0.5, 1.0))

				sum.Observe(1)
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.latency_count", nil)
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.latency_sum", nil)
				sum.Observe(3)

				p, ok := s.Read().Summary("runtime.latency", nil)
				require.True(t, ok, "expected runtime summary point")
				require.Equal(t, SampleValue(2), p.Count)
				require.Equal(t, SampleValue(4), p.Sum)
				require.Len(t, p.Quantiles, 2)
				require.False(t, math.IsNaN(p.Quantiles[0].Value) || math.IsNaN(p.Quantiles[1].Value), "expected finite quantile values: %#v", p.Quantiles)

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "runtime.latency_count", nil, 2)
				mustValue(t, fr, "runtime.latency_sum", nil, 4)
				mustDelta(t, fr, "runtime.latency_count", nil, 1)
				mustDelta(t, fr, "runtime.latency_sum", nil, 3)
				_, ok = fr.Summary("runtime.latency", nil)
				require.False(t, ok, "expected flattened view to hide typed summary getter")
			},
		},
		"runtime histogram and stateset are readable and flattenable": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				h := m.Histogram("req_duration", WithHistogramBounds(1, 2))
				ss := m.StateSet("mode", WithStateSetStates("maintenance", "operational"), WithStateSetMode(ModeEnum))

				h.Observe(0.5)
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.req_duration_bucket", Labels{"le": "1"})
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.req_duration_count", nil)
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.req_duration_sum", nil)
				h.Observe(3)
				ss.Enable("operational")

				mustHistogram(t, s.Read(), "runtime.req_duration", nil, HistogramPoint{
					Count: 2, Sum: 3.5,
					Buckets: []BucketPoint{
						{UpperBound: 1, CumulativeCount: 1},
						{UpperBound: 2, CumulativeCount: 1},
					},
				})
				mustStateSet(t, s.Read(), "runtime.mode", nil, map[string]bool{
					"maintenance": false,
					"operational": true,
				})

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "runtime.req_duration_bucket", Labels{"le": "1"}, 1)
				mustValue(t, fr, "runtime.req_duration_bucket", Labels{"le": "2"}, 0)
				mustValue(t, fr, "runtime.req_duration_bucket", Labels{"le": "+Inf"}, 1)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "1"}, 0)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "2"}, 0)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "+Inf"}, 1)
				mustDelta(t, fr, "runtime.req_duration_count", nil, 1)
				mustDelta(t, fr, "runtime.req_duration_sum", nil, 3)
				mustValue(t, fr, "runtime.mode", Labels{"runtime.mode": "maintenance"}, 0)
				mustValue(t, fr, "runtime.mode", Labels{"runtime.mode": "operational"}, 1)
			},
		},
		"runtime histogram and summary sum deltas are unavailable on decrease": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				h := m.Histogram("req_duration", WithHistogramBounds(0, 10))
				sum := m.Summary("latency")

				h.Observe(10)
				sum.Observe(10)
				h.Observe(-5)
				sum.Observe(-5)

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "runtime.req_duration_sum", nil, 5)
				mustValue(t, fr, "runtime.latency_sum", nil, 5)
				mustNoDelta(t, fr, "runtime.req_duration_sum", nil)
				mustNoDelta(t, fr, "runtime.latency_sum", nil)
				mustDelta(t, fr, "runtime.req_duration_count", nil, 1)
				mustDelta(t, fr, "runtime.latency_count", nil, 1)
			},
		},
		"runtime histogram flattened deltas survive compaction": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				view := runtimeStoreViewForTest(t, s)
				view.backend.compaction = runtimeCompactionPolicy{
					maxOverlayDepth:  1,
					maxOverlayWrites: 1,
				}

				h := s.Write().StatefulMeter("runtime").Histogram("req_duration", WithHistogramBounds(1, 2))
				h.Observe(0.5)
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.req_duration_bucket", Labels{"le": "1"})

				h.Observe(1.5)
				fr := s.Read(ReadFlatten())
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "1"}, 0)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "2"}, 1)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "+Inf"}, 0)
				mustDelta(t, fr, "runtime.req_duration_count", nil, 1)

				h.Observe(3)
				fr = s.Read(ReadFlatten())
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "1"}, 0)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "2"}, 0)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{"le": "+Inf"}, 1)
				mustDelta(t, fr, "runtime.req_duration_count", nil, 1)
			},
		},
		"runtime MeasureSet gauge and counter are readable and flattenable": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				g := m.MeasureSetGauge(
					"usage",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "value"},
						MeasureFieldSpec{Name: "limit"},
					),
				)
				c := m.MeasureSetCounter(
					"events",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "ok"},
						MeasureFieldSpec{Name: "failed"},
					),
				)

				g.SetFields(map[string]SampleValue{
					"value": 10,
					"limit": 20,
				})
				g.SetField("value", 11)
				g.AddField("limit", 2)
				mustMeasureSet(t, s.Read(), "runtime.usage", nil, []SampleValue{11, 22})
				mustValue(t, s.Read(ReadFlatten()), "runtime.usage_value", measureSetFieldLabels("value"), 11)
				mustValue(t, s.Read(ReadFlatten()), "runtime.usage_limit", measureSetFieldLabels("limit"), 22)

				c.AddFields(map[string]SampleValue{
					"ok":     5,
					"failed": 1,
				})
				mustNoDelta(t, s.Read(ReadFlatten()), "runtime.events_ok", measureSetFieldLabels("ok"))
				c.AddField("ok", 2)
				mustDelta(t, s.Read(ReadFlatten()), "runtime.events_ok", measureSetFieldLabels("ok"), 2)
				mustDelta(t, s.Read(ReadFlatten()), "runtime.events_failed", measureSetFieldLabels("failed"), 0)
				c.AddField("failed", 3)
				mustMeasureSet(t, s.Read(), "runtime.events", nil, []SampleValue{7, 4})
				mustDelta(t, s.Read(ReadFlatten()), "runtime.events_ok", measureSetFieldLabels("ok"), 0)
				mustDelta(t, s.Read(ReadFlatten()), "runtime.events_failed", measureSetFieldLabels("failed"), 3)
			},
		},
		"runtime MeasureSet flatten label key collision panics": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				ms := s.Write().StatefulMeter("runtime").
					WithLabels(Label{Key: MeasureSetFieldLabel, Value: "already-present"}).
					MeasureSetGauge(
						"usage",
						WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
					)

				expectPanic(t, func() {
					ms.SetPoint(MeasureSetPoint{Values: []SampleValue{1}})
				})
			},
		},
		"runtime counter is thread-safe for concurrent writers": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				c := s.Write().StatefulMeter("runtime").Counter("events_total")

				const workers = 8
				const perWorker = 200

				var wg sync.WaitGroup
				wg.Add(workers)
				for range workers {
					go func() {
						defer wg.Done()
						for range perWorker {
							c.Add(1)
						}
					}()
				}
				wg.Wait()

				mustValue(t, s.Read(), "runtime.events_total", nil, workers*perWorker)
			},
		},
		"runtime counter delta works with intervening writes to other metrics": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				c := m.Counter("events_total")
				g := m.Gauge("heap_bytes")

				c.Add(10)
				g.Set(5) // intervening non-counter write
				c.Add(4)

				mustValue(t, s.Read(), "runtime.events_total", nil, 14)
				mustDelta(t, s.Read(), "runtime.events_total", nil, 4)
			},
		},
		"runtime wall-clock retention evicts stale series": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				view := runtimeStoreViewForTest(t, s)
				now := time.Unix(1_700_000_000, 0)
				view.backend.now = func() time.Time { return now }
				view.backend.retention = runtimeRetentionPolicy{
					ttl:       5 * time.Second,
					maxSeries: 0,
				}
				view.backend.compaction = runtimeCompactionPolicy{
					maxOverlayDepth:  1,
					maxOverlayWrites: 1,
				}

				m := s.Write().StatefulMeter("runtime")
				g := m.Gauge("queue_depth")
				lsa := m.LabelSet(Label{Key: "id", Value: "a"})
				lsb := m.LabelSet(Label{Key: "id", Value: "b"})

				g.Set(10, lsa)
				now = now.Add(2 * time.Second)
				g.Set(20, lsb)
				mustValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "a"}, 10)
				mustValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "b"}, 20)

				now = now.Add(4 * time.Second)
				g.Set(21, lsb) // triggers retention sweep
				mustNoValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "a"})
				mustValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "b"}, 21)
			},
		},
		"runtime same-key write after TTL starts fresh baseline": {
			run: func(t *testing.T) {
				t.Run("counter", func(t *testing.T) {
					s, view, now := runtimeTTLNoCompactionStoreForTest(t)
					c := s.Write().StatefulMeter("runtime").Counter("events_total")

					c.Add(10)
					*now = (*now).Add(6 * time.Second)
					c.Add(4)

					mustValue(t, s.Read(), "runtime.events_total", nil, 4)
					mustNoDelta(t, s.Read(), "runtime.events_total", nil)
					forceRuntimeCompactionForTest(t, s, view)
					mustValue(t, s.Read(), "runtime.events_total", nil, 4)
					mustNoDelta(t, s.Read(), "runtime.events_total", nil)
				})

				t.Run("histogram", func(t *testing.T) {
					s, view, now := runtimeTTLNoCompactionStoreForTest(t)
					h := s.Write().StatefulMeter("runtime").Histogram("req_duration", WithHistogramBounds(1, 2))

					h.Observe(0.5)
					*now = (*now).Add(6 * time.Second)
					h.Observe(3)

					assertFreshRuntimeHistogramAfterTTL(t, s)
					forceRuntimeCompactionForTest(t, s, view)
					assertFreshRuntimeHistogramAfterTTL(t, s)
				})

				t.Run("summary sketch", func(t *testing.T) {
					s, view, now := runtimeTTLNoCompactionStoreForTest(t)
					sum := s.Write().StatefulMeter("runtime").Summary("latency", WithSummaryQuantiles(0.5))

					sum.Observe(1)
					*now = (*now).Add(6 * time.Second)
					sum.Observe(9)

					assertFreshRuntimeSummaryAfterTTL(t, s)
					forceRuntimeCompactionForTest(t, s, view)
					assertFreshRuntimeSummaryAfterTTL(t, s)
				})

				t.Run("measureset counter", func(t *testing.T) {
					s, view, now := runtimeTTLNoCompactionStoreForTest(t)
					ms := s.Write().StatefulMeter("runtime").MeasureSetCounter(
						"events",
						WithMeasureSetFields(
							MeasureFieldSpec{Name: "ok"},
							MeasureFieldSpec{Name: "failed"},
						),
					)

					ms.AddFields(map[string]SampleValue{"ok": 5, "failed": 1})
					*now = (*now).Add(6 * time.Second)
					ms.AddField("failed", 3)

					assertFreshRuntimeMeasureSetCounterAfterTTL(t, s)
					forceRuntimeCompactionForTest(t, s, view)
					assertFreshRuntimeMeasureSetCounterAfterTTL(t, s)
				})

				t.Run("measureset gauge", func(t *testing.T) {
					s, view, now := runtimeTTLNoCompactionStoreForTest(t)
					ms := s.Write().StatefulMeter("runtime").MeasureSetGauge(
						"usage",
						WithMeasureSetFields(
							MeasureFieldSpec{Name: "value"},
							MeasureFieldSpec{Name: "limit"},
						),
					)

					ms.SetFields(map[string]SampleValue{"value": 10, "limit": 20})
					*now = (*now).Add(6 * time.Second)
					ms.AddField("value", 3)

					mustMeasureSet(t, s.Read(), "runtime.usage", nil, []SampleValue{3, 0})
					mustValue(t, s.Read(ReadFlatten()), "runtime.usage_value", measureSetFieldLabels("value"), 3)
					mustValue(t, s.Read(ReadFlatten()), "runtime.usage_limit", measureSetFieldLabels("limit"), 0)
					forceRuntimeCompactionForTest(t, s, view)
					mustMeasureSet(t, s.Read(), "runtime.usage", nil, []SampleValue{3, 0})
					mustValue(t, s.Read(ReadFlatten()), "runtime.usage_value", measureSetFieldLabels("value"), 3)
					mustValue(t, s.Read(ReadFlatten()), "runtime.usage_limit", measureSetFieldLabels("limit"), 0)
				})

				t.Run("stateset", func(t *testing.T) {
					s, view, now := runtimeTTLNoCompactionStoreForTest(t)
					ss := s.Write().StatefulMeter("runtime").StateSet(
						"mode",
						WithStateSetStates("maintenance", "operational"),
						WithStateSetMode(ModeEnum),
					)

					ss.Enable("operational")
					*now = (*now).Add(6 * time.Second)
					ss.Enable("maintenance")

					mustStateSet(t, s.Read(), "runtime.mode", nil, map[string]bool{
						"maintenance": true,
						"operational": false,
					})
					mustValue(t, s.Read(ReadFlatten()), "runtime.mode", Labels{"runtime.mode": "maintenance"}, 1)
					mustValue(t, s.Read(ReadFlatten()), "runtime.mode", Labels{"runtime.mode": "operational"}, 0)
					forceRuntimeCompactionForTest(t, s, view)
					mustStateSet(t, s.Read(), "runtime.mode", nil, map[string]bool{
						"maintenance": true,
						"operational": false,
					})
					mustValue(t, s.Read(ReadFlatten()), "runtime.mode", Labels{"runtime.mode": "maintenance"}, 1)
					mustValue(t, s.Read(ReadFlatten()), "runtime.mode", Labels{"runtime.mode": "operational"}, 0)
				})
			},
		},
		"runtime same-key write before TTL preserves baseline": {
			run: func(t *testing.T) {
				s, _, now := runtimeTTLNoCompactionStoreForTest(t)
				m := s.Write().StatefulMeter("runtime")
				c := m.Counter("events_total")
				h := m.Histogram("req_duration", WithHistogramBounds(1, 2))
				sum := m.Summary("latency", WithSummaryQuantiles(0.5))
				ms := m.MeasureSetCounter(
					"events",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "ok"},
						MeasureFieldSpec{Name: "failed"},
					),
				)

				c.Add(10)
				h.Observe(0.5)
				sum.Observe(1)
				ms.AddFields(map[string]SampleValue{"ok": 5, "failed": 1})

				*now = (*now).Add(2 * time.Second)
				c.Add(4)
				h.Observe(3)
				sum.Observe(9)
				ms.AddField("failed", 3)

				mustValue(t, s.Read(), "runtime.events_total", nil, 14)
				mustDelta(t, s.Read(), "runtime.events_total", nil, 4)

				fr := s.Read(ReadFlatten())
				mustValue(t, fr, "runtime.req_duration_count", nil, 2)
				mustValue(t, fr, "runtime.req_duration_sum", nil, 3.5)
				mustDelta(t, fr, "runtime.req_duration_bucket", Labels{HistogramBucketLabel: "+Inf"}, 1)
				mustDelta(t, fr, "runtime.req_duration_count", nil, 1)

				point, ok := s.Read().Summary("runtime.latency", nil)
				require.True(t, ok, "expected runtime summary point")
				require.Equal(t, SampleValue(2), point.Count)
				require.Equal(t, SampleValue(10), point.Sum)
				require.Len(t, point.Quantiles, 1)
				require.Equal(t, SampleValue(5), point.Quantiles[0].Value)
				mustDelta(t, fr, "runtime.latency_count", nil, 1)
				mustDelta(t, fr, "runtime.latency_sum", nil, 9)

				mustMeasureSet(t, s.Read(), "runtime.events", nil, []SampleValue{5, 4})
				mustDelta(t, fr, "runtime.events_ok", measureSetFieldLabels("ok"), 0)
				mustDelta(t, fr, "runtime.events_failed", measureSetFieldLabels("failed"), 3)
			},
		},
		"runtime max-series cap evicts oldest series deterministically": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				view := runtimeStoreViewForTest(t, s)
				now := time.Unix(1_700_000_000, 0)
				view.backend.now = func() time.Time { return now }
				view.backend.retention = runtimeRetentionPolicy{
					ttl:       0,
					maxSeries: 2,
				}
				view.backend.compaction = runtimeCompactionPolicy{
					maxOverlayDepth:  1,
					maxOverlayWrites: 1,
				}

				m := s.Write().StatefulMeter("runtime")
				g := m.Gauge("queue_depth")
				lsa := m.LabelSet(Label{Key: "id", Value: "a"})
				lsb := m.LabelSet(Label{Key: "id", Value: "b"})
				lsc := m.LabelSet(Label{Key: "id", Value: "c"})

				g.Set(10, lsa)
				g.Set(20, lsb)
				g.Set(30, lsc)

				mustNoValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "a"})
				mustValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "b"}, 20)
				mustValue(t, s.Read(ReadRaw()), "runtime.queue_depth", Labels{"id": "c"}, 30)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func runtimeStoreViewForTest(t *testing.T, s RuntimeStore) *runtimeStoreView {
	t.Helper()
	v, ok := s.(*runtimeStoreView)
	require.True(t, ok, "unexpected runtime store implementation: %T", s)
	return v
}

func runtimeTTLNoCompactionStoreForTest(t *testing.T) (RuntimeStore, *runtimeStoreView, *time.Time) {
	t.Helper()
	s := NewRuntimeStore()
	view := runtimeStoreViewForTest(t, s)
	now := time.Unix(1_700_000_000, 0)
	view.backend.now = func() time.Time { return now }
	view.backend.retention = runtimeRetentionPolicy{
		ttl:       5 * time.Second,
		maxSeries: 0,
	}
	view.backend.compaction = runtimeCompactionPolicy{}
	return s, view, &now
}

func forceRuntimeCompactionForTest(t *testing.T, s RuntimeStore, view *runtimeStoreView) {
	t.Helper()
	view.backend.compaction = runtimeCompactionPolicy{
		maxOverlayDepth:  1,
		maxOverlayWrites: 1,
	}
	s.Write().StatefulMeter("runtime").Gauge("compaction_trigger").Set(1)
}

func assertFreshRuntimeHistogramAfterTTL(t *testing.T, s RuntimeStore) {
	t.Helper()
	mustHistogram(t, s.Read(), "runtime.req_duration", nil, HistogramPoint{
		Count: 1,
		Sum:   3,
		Buckets: []BucketPoint{
			{UpperBound: 1, CumulativeCount: 0},
			{UpperBound: 2, CumulativeCount: 0},
		},
	})
	fr := s.Read(ReadFlatten())
	mustValue(t, fr, "runtime.req_duration_bucket", Labels{HistogramBucketLabel: "1"}, 0)
	mustValue(t, fr, "runtime.req_duration_bucket", Labels{HistogramBucketLabel: "2"}, 0)
	mustValue(t, fr, "runtime.req_duration_bucket", Labels{HistogramBucketLabel: "+Inf"}, 1)
	mustValue(t, fr, "runtime.req_duration_count", nil, 1)
	mustValue(t, fr, "runtime.req_duration_sum", nil, 3)
	mustNoDelta(t, fr, "runtime.req_duration_bucket", Labels{HistogramBucketLabel: "+Inf"})
	mustNoDelta(t, fr, "runtime.req_duration_count", nil)
	mustNoDelta(t, fr, "runtime.req_duration_sum", nil)
}

func assertFreshRuntimeSummaryAfterTTL(t *testing.T, s RuntimeStore) {
	t.Helper()
	point, ok := s.Read().Summary("runtime.latency", nil)
	require.True(t, ok, "expected runtime summary point")
	require.Equal(t, SampleValue(1), point.Count)
	require.Equal(t, SampleValue(9), point.Sum)
	require.Len(t, point.Quantiles, 1)
	require.Equal(t, SampleValue(9), point.Quantiles[0].Value)

	fr := s.Read(ReadFlatten())
	mustValue(t, fr, "runtime.latency_count", nil, 1)
	mustValue(t, fr, "runtime.latency_sum", nil, 9)
	mustNoDelta(t, fr, "runtime.latency_count", nil)
	mustNoDelta(t, fr, "runtime.latency_sum", nil)
}

func assertFreshRuntimeMeasureSetCounterAfterTTL(t *testing.T, s RuntimeStore) {
	t.Helper()
	mustMeasureSet(t, s.Read(), "runtime.events", nil, []SampleValue{0, 3})
	fr := s.Read(ReadFlatten())
	mustValue(t, fr, "runtime.events_ok", measureSetFieldLabels("ok"), 0)
	mustValue(t, fr, "runtime.events_failed", measureSetFieldLabels("failed"), 3)
	mustNoDelta(t, fr, "runtime.events_ok", measureSetFieldLabels("ok"))
	mustNoDelta(t, fr, "runtime.events_failed", measureSetFieldLabels("failed"))
}
