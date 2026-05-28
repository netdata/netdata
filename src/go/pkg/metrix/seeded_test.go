// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"
	"time"
)

func TestSeededHelperScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"SeededGauge creates visible zero-valued series": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				_ = SeededGauge(m, "queue_depth")

				mustValue(t, s.Read(ReadRaw()), "runtime.queue_depth", nil, 0)
			},
		},
		"SeededCounter creates visible zero-valued series without initial delta": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				_ = SeededCounter(m, "jobs_total")

				mustValue(t, s.Read(ReadRaw()), "runtime.jobs_total", nil, 0)
				mustNoDelta(t, s.Read(ReadRaw()), "runtime.jobs_total", nil)
			},
		},
		"SeededCounter accumulates normally after seed": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				c := SeededCounter(m, "events_total")

				c.Add(5)
				mustValue(t, s.Read(ReadRaw()), "runtime.events_total", nil, 5)
				mustDelta(t, s.Read(ReadRaw()), "runtime.events_total", nil, 5)
			},
		},
		"Seeded helpers preserve meter labels": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime").WithLabels(Label{Key: "component", Value: "functions"})
				_ = SeededGauge(m, "invocations_active")
				_ = SeededCounter(m, "calls_total")

				labels := Labels{"component": "functions"}
				mustValue(t, s.Read(ReadRaw()), "runtime.invocations_active", labels, 0)
				mustValue(t, s.Read(ReadRaw()), "runtime.calls_total", labels, 0)
			},
		},
		"Seeded series can be evicted by TTL when compaction is triggered later": {
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
				_ = SeededCounter(m, "stale_total")
				mustValue(t, s.Read(ReadRaw()), "runtime.stale_total", nil, 0)

				now = now.Add(6 * time.Second)
				SeededGauge(m, "trigger")
				mustNoValue(t, s.Read(ReadRaw()), "runtime.stale_total", nil)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
