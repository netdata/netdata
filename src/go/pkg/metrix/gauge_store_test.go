// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

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

				if _, ok := s.Read().Value("apache.workers_busy", nil); ok {
					t.Fatalf("expected snapshot gauge hidden after successful cycle with no sample")
				}
				mustValue(t, s.ReadRaw(), "apache.workers_busy", nil, 10)
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
				if meta.LastAttemptStatus != CollectStatusFailed || meta.LastAttemptSeq != 2 || meta.LastSuccessSeq != 1 {
					t.Fatalf("unexpected collect meta after abort: %#v", meta)
				}
				if _, ok := s.Read().Value("svc.load", nil); ok {
					t.Fatalf("expected snapshot gauge hidden after failed attempt")
				}
				mustValue(t, s.ReadRaw(), "svc.load", nil, 11)
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
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
