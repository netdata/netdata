// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegistrationTransactionality pins the staged-registration contract:
// a registration made during an active cycle is provisional - it joins the
// committed registry only when the cycle commits, and is discarded if the cycle
// aborts. Conflict handling is unchanged at this step (fail-loud); the
// commit-time state model arrives in a later step.
func TestRegistrationTransactionality(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"registration in a committed cycle persists (later incompatible re-declare conflicts)": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				// svc.m is now a committed gauge; re-declaring it as a counter must conflict.
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m")
				})
			},
		},
		"registration in an aborted cycle is discarded (later different-kind re-declare is clean)": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				_ = s.Write().SnapshotMeter("svc").Gauge("m")
				cc.AbortCycle()

				// The gauge registration was never committed, so svc.m is free to be
				// declared as a different kind without conflict.
				require.NotPanics(t, func() {
					_ = s.Write().SnapshotMeter("svc").Counter("m")
				})
			},
		},
		"duplicate compatible registration within a cycle dedups": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				require.NotPanics(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("m")
					_ = s.Write().SnapshotMeter("svc").Gauge("m")
				})
				cc.AbortCycle()
			},
		},
		"re-declare compatible in a later cycle dedups against the committed registry": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				s.Write().SnapshotMeter("svc").Gauge("m").Observe(1)
				require.NoError(t, cc.CommitCycleSuccess())

				cc.BeginCycle()
				require.NotPanics(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("m")
				})
				cc.AbortCycle()
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
