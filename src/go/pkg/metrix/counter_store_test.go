// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCounterStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot counter delta and reset-aware semantics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				c := s.Write().SnapshotMeter("http").Counter("requests_total")

				cc.BeginCycle()
				c.ObserveTotal(100)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "http.requests_total", nil, 100)
				mustNoDelta(t, s.Read(), "http.requests_total", nil)

				cc.BeginCycle()
				c.ObserveTotal(150)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "http.requests_total", nil, 150)
				mustDelta(t, s.Read(), "http.requests_total", nil, 50)

				cc.BeginCycle()
				c.ObserveTotal(20)
				cc.CommitCycleSuccess()
				mustDelta(t, s.Read(), "http.requests_total", nil, 20)
			},
		},
		"snapshot counter delta unavailable on attempt gap": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				c := s.Write().SnapshotMeter("db").Counter("queries_total")

				cc.BeginCycle()
				c.ObserveTotal(10)
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				c.ObserveTotal(20)
				cc.CommitCycleSuccess()
				mustDelta(t, s.Read(), "db.queries_total", nil, 10)

				cc.BeginCycle()
				c.ObserveTotal(30)
				cc.AbortCycle()

				cc.BeginCycle()
				c.ObserveTotal(40)
				cc.CommitCycleSuccess()
				mustNoDelta(t, s.Read(), "db.queries_total", nil)
			},
		},
		"snapshot counter repeated writes are last-write-wins in cycle": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				c := s.Write().SnapshotMeter("app").Counter("events_total")

				cc.BeginCycle()
				c.ObserveTotal(10)
				c.ObserveTotal(12)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "app.events_total", nil, 12)
			},
		},
		"stateful counter add baselines from committed and accumulates": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				c := s.Write().StatefulMeter("runtime").Counter("jobs_total")

				cc.BeginCycle()
				c.Add(5)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.jobs_total", nil, 5)
				mustNoDelta(t, s.Read(), "runtime.jobs_total", nil)

				cc.BeginCycle()
				c.Add(2)
				c.Add(3)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "runtime.jobs_total", nil, 10)
				mustDelta(t, s.Read(), "runtime.jobs_total", nil, 5)
			},
		},
		"stateful counter negative add panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				c := s.Write().StatefulMeter("runtime").Counter("bad_total")

				cc.BeginCycle()
				expectPanic(t, func() {
					c.Add(-1)
				})
				cc.AbortCycle()
			},
		},
		"counter mode mixing snapshot and stateful panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				s.Write().SnapshotMeter("mixed").Counter("metric")
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("mixed").Counter("metric")
				})
			},
		},
		"delta on non-counter returns unavailable": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("g").Gauge("value")
				cc.BeginCycle()
				g.Observe(3)
				cc.CommitCycleSuccess()
				mustNoDelta(t, s.Read(), "g.value", nil)
			},
		},
		"snapshot counter hidden from Read on failed latest attempt": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				c := s.Write().SnapshotMeter("svc").Counter("requests_total")

				cc.BeginCycle()
				c.ObserveTotal(9)
				cc.CommitCycleSuccess()
				mustValue(t, s.Read(), "svc.requests_total", nil, 9)

				cc.BeginCycle()
				c.ObserveTotal(11)
				cc.AbortCycle()

				_, ok := s.Read().Value("svc.requests_total", nil)
				require.False(t, ok, "expected snapshot counter hidden after failed attempt")
				mustValue(t, s.Read(ReadRaw()), "svc.requests_total", nil, 9)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
