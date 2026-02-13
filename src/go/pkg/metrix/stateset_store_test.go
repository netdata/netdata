// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

func TestStateSetStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot stateset read and flatten": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().SnapshotMeter("svc").StateSet(
					"mode",
					WithStateSetStates("maintenance", "operational", "recovery"),
					WithStateSetMode(ModeEnum),
				)

				cc.BeginCycle()
				ss.Enable("operational")
				cc.CommitCycleSuccess()

				mustStateSet(t, s.Read(), "svc.mode", nil, map[string]bool{
					"maintenance": false,
					"operational": true,
					"recovery":    false,
				})
				if _, ok := s.Read().Value("svc.mode", nil); ok {
					t.Fatalf("expected non-scalar stateset to be unavailable via Value")
				}

				fr := s.Read().Flatten()
				mustValue(t, fr, "svc.mode", Labels{"svc.mode": "maintenance"}, 0)
				mustValue(t, fr, "svc.mode", Labels{"svc.mode": "operational"}, 1)
				mustValue(t, fr, "svc.mode", Labels{"svc.mode": "recovery"}, 0)
			},
		},
		"snapshot stateset freshness hides stale series from Read but not ReadRaw": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().SnapshotMeter("svc").StateSet(
					"mode",
					WithStateSetStates("maintenance", "operational"),
					WithStateSetMode(ModeEnum),
				)

				cc.BeginCycle()
				ss.Enable("operational")
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				cc.CommitCycleSuccess()

				if _, ok := s.Read().StateSet("svc.mode", nil); ok {
					t.Fatalf("expected stale snapshot stateset hidden from Read")
				}
				mustStateSet(t, s.ReadRaw(), "svc.mode", nil, map[string]bool{
					"maintenance": false,
					"operational": true,
				})

				if _, ok := s.Read().Flatten().Value("svc.mode", Labels{"svc.mode": "operational"}); ok {
					t.Fatalf("expected stale snapshot stateset flattened series hidden from Read().Flatten()")
				}
				mustValue(t, s.ReadRaw().Flatten(), "svc.mode", Labels{"svc.mode": "operational"}, 1)
			},
		},
		"stateful stateset remains visible across cycles without writes": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().StatefulMeter("svc").StateSet(
					"feature_flags",
					WithStateSetStates("a", "b", "c"),
				)

				cc.BeginCycle()
				ss.ObserveStateSet(StateSetPoint{States: map[string]bool{"a": true, "b": true}})
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				cc.CommitCycleSuccess()

				mustStateSet(t, s.Read(), "svc.feature_flags", nil, map[string]bool{
					"a": true,
					"b": true,
					"c": false,
				})
				mustValue(t, s.Read().Flatten(), "svc.feature_flags", Labels{"svc.feature_flags": "a"}, 1)
				mustValue(t, s.Read().Flatten(), "svc.feature_flags", Labels{"svc.feature_flags": "b"}, 1)
				mustValue(t, s.Read().Flatten(), "svc.feature_flags", Labels{"svc.feature_flags": "c"}, 0)
			},
		},
		"stateset enum mode validation panics on invalid active count": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().SnapshotMeter("svc").StateSet(
					"mode",
					WithStateSetStates("maintenance", "operational"),
					WithStateSetMode(ModeEnum),
				)

				cc.BeginCycle()
				expectPanic(t, func() {
					ss.Enable("maintenance", "operational")
				})
				cc.AbortCycle()
			},
		},
		"stateset observe panics on undeclared state": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().SnapshotMeter("svc").StateSet(
					"mode",
					WithStateSetStates("maintenance", "operational"),
				)

				cc.BeginCycle()
				expectPanic(t, func() {
					ss.ObserveStateSet(StateSetPoint{States: map[string]bool{"unknown": true}})
				})
				cc.AbortCycle()
			},
		},
		"stateset declaration requires WithStateSetStates": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").StateSet("mode")
				})
			},
		},
		"stateset schema mismatch panics on redeclaration": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").StateSet(
					"mode",
					WithStateSetStates("maintenance", "operational"),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").StateSet(
						"mode",
						WithStateSetStates("maintenance", "recovery"),
					)
				})
			},
		},
		"flatten label key collision panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().SnapshotMeter("svc").
					WithLabels(Label{Key: "svc.mode", Value: "already-present"}).
					StateSet("mode", WithStateSetStates("maintenance", "operational"))

				cc.BeginCycle()
				expectPanic(t, func() {
					ss.Enable("maintenance")
				})
				cc.AbortCycle()
			},
		},
		"stateset read returns a copy": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ss := s.Write().SnapshotMeter("svc").StateSet("mode", WithStateSetStates("a", "b"))

				cc.BeginCycle()
				ss.ObserveStateSet(StateSetPoint{States: map[string]bool{"a": true}})
				cc.CommitCycleSuccess()

				p, ok := s.Read().StateSet("svc.mode", nil)
				if !ok {
					t.Fatalf("expected stateset point")
				}
				p.States["a"] = false

				mustStateSet(t, s.Read(), "svc.mode", nil, map[string]bool{"a": true, "b": false})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func mustStateSet(t *testing.T, r Reader, name string, labels Labels, want map[string]bool) {
	t.Helper()
	got, ok := r.StateSet(name, labels)
	if !ok {
		t.Fatalf("expected stateset for %s", name)
	}
	if len(got.States) != len(want) {
		t.Fatalf("unexpected stateset size for %s: got=%d want=%d", name, len(got.States), len(want))
	}
	for k, w := range want {
		if g, ok := got.States[k]; !ok || g != w {
			t.Fatalf("unexpected stateset state %s for %s: got=%v want=%v", k, name, g, w)
		}
	}
}
