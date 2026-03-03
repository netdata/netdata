// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"testing"
)

func TestValueValidationScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"collector gauge rejects NaN and Inf": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("svc").Gauge("load")

				cc.BeginCycle()
				expectPanic(t, func() { g.Observe(math.NaN()) })
				cc.AbortCycle()

				cc.BeginCycle()
				expectPanic(t, func() { g.Observe(math.Inf(1)) })
				cc.AbortCycle()
			},
		},
		"collector counter rejects NaN and Inf": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sc := s.Write().SnapshotMeter("svc").Counter("total")
				st := s.Write().StatefulMeter("svc").Counter("total_stateful")

				cc.BeginCycle()
				expectPanic(t, func() { sc.ObserveTotal(math.NaN()) })
				cc.AbortCycle()

				cc.BeginCycle()
				expectPanic(t, func() { st.Add(math.Inf(1)) })
				cc.AbortCycle()
			},
		},
		"collector summary and histogram reject non-finite samples": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sum := s.Write().StatefulMeter("svc").Summary("latency")
				hist := s.Write().StatefulMeter("svc").Histogram("duration", WithHistogramBounds(1, 2))

				cc.BeginCycle()
				expectPanic(t, func() { sum.Observe(math.Inf(1)) })
				cc.AbortCycle()

				cc.BeginCycle()
				expectPanic(t, func() { hist.Observe(math.NaN()) })
				cc.AbortCycle()
			},
		},
		"collector point APIs reject non-finite values": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				sum := s.Write().SnapshotMeter("svc").Summary("latency", WithSummaryQuantiles(0.5))
				hist := s.Write().SnapshotMeter("svc").Histogram("duration", WithHistogramBounds(1, 2))

				cc.BeginCycle()
				expectPanic(t, func() {
					sum.ObservePoint(SummaryPoint{
						Count: 1,
						Sum:   1,
						Quantiles: []QuantilePoint{
							{Quantile: 0.5, Value: math.NaN()},
						},
					})
				})
				cc.AbortCycle()

				cc.BeginCycle()
				expectPanic(t, func() {
					hist.ObservePoint(HistogramPoint{
						Count: 1,
						Sum:   math.Inf(1),
						Buckets: []BucketPoint{
							{UpperBound: 1, CumulativeCount: 1},
							{UpperBound: 2, CumulativeCount: 1},
						},
					})
				})
				cc.AbortCycle()
			},
		},
		"runtime scalar writes reject NaN and Inf": {
			run: func(t *testing.T) {
				s := NewRuntimeStore()
				m := s.Write().StatefulMeter("runtime")
				g := m.Gauge("heap")
				c := m.Counter("events_total")
				h := m.Histogram("latency", WithHistogramBounds(1, 2))
				sum := m.Summary("duration")

				expectPanic(t, func() { g.Set(math.NaN()) })
				expectPanic(t, func() { c.Add(math.Inf(1)) })
				expectPanic(t, func() { h.Observe(math.Inf(1)) })
				expectPanic(t, func() { sum.Observe(math.NaN()) })
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
