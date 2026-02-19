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
				mustValue(t, fr, "runtime.req_duration_bucket", Labels{"le": "2"}, 1)
				mustValue(t, fr, "runtime.req_duration_bucket", Labels{"le": "+Inf"}, 2)
				mustValue(t, fr, "runtime.mode", Labels{"runtime.mode": "maintenance"}, 0)
				mustValue(t, fr, "runtime.mode", Labels{"runtime.mode": "operational"}, 1)
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
				for i := 0; i < workers; i++ {
					go func() {
						defer wg.Done()
						for j := 0; j < perWorker; j++ {
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
