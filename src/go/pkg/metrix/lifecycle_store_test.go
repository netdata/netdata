// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

func TestLifecycleStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"double BeginCycle panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				cc.BeginCycle()
				expectPanic(t, func() { cc.BeginCycle() })
				cc.AbortCycle()
			},
		},
		"CommitCycleSuccess without BeginCycle panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				expectPanic(t, func() { cc.CommitCycleSuccess() })
			},
		},
		"AbortCycle without BeginCycle panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				expectPanic(t, func() { cc.AbortCycle() })
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
