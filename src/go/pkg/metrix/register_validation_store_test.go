// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

func TestRegisterInstrumentValidationScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"summary quantiles on gauge panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("load", WithSummaryQuantiles(0.5))
				})
			},
		},
		"histogram bounds on summary panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Summary("latency", WithHistogramBounds(1))
				})
			},
		},
		"stateset options on non-stateset panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("requests_total", WithStateSetStates("up", "down"))
				})
			},
		},
		"window option on stateful gauge panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("svc").Gauge("heap", WithWindow(WindowCycle))
				})
			},
		},
		"snapshot gauge freshness committed panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("load", WithFreshness(FreshnessCommitted))
				})
			},
		},
		"window cycle with explicit freshness committed panic": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("svc").Summary(
						"latency",
						WithSummaryQuantiles(0.5),
						WithWindow(WindowCycle),
						WithFreshness(FreshnessCommitted),
					)
				})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
