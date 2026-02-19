// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGaugeStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot gauge freshness and raw visibility": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("apache").Gauge("workers_busy")

				cc.BeginCycle()
				g.Observe(10)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "apache.workers_busy", nil, 10)

				cc.BeginCycle()
				cc.CommitCycleSuccess()

				_, ok := s.Read().Value("apache.workers_busy", nil)
				require.False(t, ok, "expected snapshot gauge hidden after successful cycle with no sample")
				mustValue(t, s.Read(ReadRaw()), "apache.workers_busy", nil, 10)
			},
		},
		"stateful gauge set and add semantics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().StatefulMeter("runtime").Gauge("heap_bytes")

				cc.BeginCycle()
				g.Set(5)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.heap_bytes", nil, 5)

				cc.BeginCycle()
				g.Add(2)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.heap_bytes", nil, 7)

				cc.BeginCycle()
				g.Add(3)
				g.Add(4)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.heap_bytes", nil, 14)

				cc.BeginCycle()
				g.Set(1)
				g.Add(2)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.heap_bytes", nil, 3)

				cc.BeginCycle()
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.heap_bytes", nil, 3)
			},
		},
		"abort keeps committed data and marks failed attempt": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("svc").Gauge("load")

				cc.BeginCycle()
				g.Observe(11)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "svc.load", nil, 11)

				cc.BeginCycle()
				g.Observe(20)
				cc.AbortCycle()

				meta := s.Read().CollectMeta()
				require.Equal(t, CollectStatusFailed, meta.LastAttemptStatus, "unexpected collect meta after abort: %#v", meta)
				require.Equal(t, uint64(2), meta.LastAttemptSeq, "unexpected collect meta after abort: %#v", meta)
				require.Equal(t, uint64(1), meta.LastSuccessSeq, "unexpected collect meta after abort: %#v", meta)
				_, ok := s.Read().Value("svc.load", nil)
				require.False(t, ok, "expected snapshot gauge hidden after failed attempt")
				mustValue(t, s.Read(ReadRaw()), "svc.load", nil, 11)
			},
		},
		"commit updates collect metadata on successful cycles": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("svc").Gauge("load")

				cc.BeginCycle()
				g.Observe(3)
				cc.CommitCycleSuccess()

				meta := s.Read().CollectMeta()
				require.Equal(t, CollectStatusSuccess, meta.LastAttemptStatus, "unexpected collect meta after first success: %#v", meta)
				require.Equal(t, uint64(1), meta.LastAttemptSeq, "unexpected collect meta after first success: %#v", meta)
				require.Equal(t, uint64(1), meta.LastSuccessSeq, "unexpected collect meta after first success: %#v", meta)

				cc.BeginCycle()
				g.Observe(4)
				cc.CommitCycleSuccess()

				meta = s.Read().CollectMeta()
				require.Equal(t, CollectStatusSuccess, meta.LastAttemptStatus, "unexpected collect meta after second success: %#v", meta)
				require.Equal(t, uint64(2), meta.LastAttemptSeq, "unexpected collect meta after second success: %#v", meta)
				require.Equal(t, uint64(2), meta.LastSuccessSeq, "unexpected collect meta after second success: %#v", meta)
			},
		},
		"mode mixing snapshot and stateful panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				s.Write().SnapshotMeter("mixed").Gauge("metric")
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("mixed").Gauge("metric")
				})
			},
		},
		"write outside cycle panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				g := s.Write().SnapshotMeter("panic").Gauge("outside")
				expectPanic(t, func() {
					g.Observe(1)
				})
			},
		},
		"label set validation and merging": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sm := s.Write().SnapshotMeter("apache").WithLabels(Label{Key: "instance", Value: "a"})
				g := sm.Gauge("workers")

				cc.BeginCycle()
				g.Observe(3)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "apache.workers", Labels{"instance": "a"}, 3)

				cc.BeginCycle()
				ls := sm.LabelSet(Label{Key: "role", Value: "primary"})
				g.Observe(9, ls)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "apache.workers", Labels{"instance": "a", "role": "primary"}, 9)

				cc.BeginCycle()
				dup := sm.LabelSet(Label{Key: "instance", Value: "b"})
				expectPanic(t, func() {
					g.Observe(1, dup)
				})
				cc.AbortCycle()
			},
		},
		"foreign label set panics": {
			run: func(t *testing.T) {
				s1 := NewCollectorStore()
				s2 := NewCollectorStore()
				cc := cycleController(t, s1)
				g := s1.Write().SnapshotMeter("svc").Gauge("reqs")
				foreign := s2.Write().SnapshotMeter("svc").LabelSet(Label{Key: "instance", Value: "x"})

				cc.BeginCycle()
				expectPanic(t, func() {
					g.Observe(1, foreign)
				})
				cc.AbortCycle()
			},
		},
		"published snapshot labels stay stable across later commits": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sm := s.Write().SnapshotMeter("apache").WithLabels(Label{Key: "instance", Value: "a"})
				g := sm.Gauge("workers")
				role := sm.LabelSet(Label{Key: "role", Value: "primary"})

				cc.BeginCycle()
				g.Observe(10, role)
				cc.CommitCycleSuccess()

				oldReader := s.Read(ReadRaw())
				assertSingleSeries := func(r Reader, wantValue SampleValue, wantLabels map[string]string) {
					count := 0
					r.ForEachByName("apache.workers", func(labels LabelView, v SampleValue) {
						count++
						require.Equal(t, wantValue, v)
						require.Equal(t, wantLabels, labels.CloneMap())
					})
					require.Equal(t, 1, count, "unexpected series count")
				}

				assertSingleSeries(oldReader, 10, map[string]string{
					"instance": "a",
					"role":     "primary",
				})

				cc.BeginCycle()
				g.Observe(20, role)
				cc.CommitCycleSuccess()

				// Old reader snapshot must stay immutable after later commits.
				assertSingleSeries(oldReader, 10, map[string]string{
					"instance": "a",
					"role":     "primary",
				})

				assertSingleSeries(s.Read(ReadRaw()), 20, map[string]string{
					"instance": "a",
					"role":     "primary",
				})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
