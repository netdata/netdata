// SPDX-License-Identifier: GPL-3.0-or-later

// Characterization tests: executable pins of scheduler-lane contracts,
// written before the per-config-lane redesign.
//
//   - MUST-NOT-FLIP: a schedule lane is owned by an invocation until its
//     TERMINAL response is finalized (worker return is not completion); the
//     next same-lane invocation is dispatched only after finalization.
//   - FLIPS-BY-DESIGN: today's lane granularity for prefix registrations is
//     the MATCHED PREFIX, so requests for two DIFFERENT resource IDs behind
//     one prefix serialize end-to-end. The redesign re-keys prefix lanes
//     per resource ID; when that lands, the cross-ID part of this pin must be
//     inverted deliberately, while the hold-until-finalize part must survive.

package functions

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCharacterization_PrefixLaneHeldUntilTerminalFinalize(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer)
	}{
		"same prefix lane stays owned until terminal finalization": {
			run: func(t *testing.T, mgr *Manager, in *chanInput, out *safeBuffer) {
				var mu sync.Mutex
				var handled []string
				mgr.RegisterPrefix("config", "test:collector:", func(fn Function) {
					mu.Lock()
					handled = append(handled, fn.UID)
					mu.Unlock()
					// Handoff-style handler: returns WITHOUT emitting a terminal
					// response (the real dyncfg handler hands off to jobmgr, which
					// responds later through the finalizer).
				})

				handledUIDs := func() []string {
					mu.Lock()
					defer mu.Unlock()
					return append([]string(nil), handled...)
				}
				hasHandled := func(uid string) func() bool {
					return func() bool {
						return slices.Contains(handledUIDs(), uid)
					}
				}

				cancel, done := startFlowManager(t, mgr)
				defer cancel()

				in.ch <- functionLine("tx1", "config test:collector:success:a enable")
				waitForCondition(t, time.Second, hasHandled("tx1"), "tx1 dispatched to handler")

				// Different config ID, same prefix: with prefix-keyed lanes it must NOT
				// be dispatched while tx1 awaits its terminal response.
				in.ch <- functionLine("tx2", "config test:collector:other:b enable")
				require.Never(t, hasHandled("tx2"), 200*time.Millisecond, 10*time.Millisecond,
					"same-prefix request for a different config ID was dispatched while the lane owner "+
						"awaited its terminal response; prefix-lane serialization no longer holds - "+
						"update the FLIPS-BY-DESIGN part of this pin deliberately, but the lane must "+
						"still be held until terminal finalize PER KEY")

				// Terminal finalization of tx1 (not its handler return) advances the lane.
				mgr.respUID("tx1", 200, "ok")
				waitForCondition(t, time.Second, hasHandled("tx2"), "tx2 dispatched after tx1 finalized")

				mgr.respUID("tx2", 200, "ok")
				waitForSubstring(t, out.String, "FUNCTION_RESULT_BEGIN tx2 200", time.Second)

				// Terminal-once: exactly one result per UID.
				assert.Equal(t, []string{"tx1", "tx2"}, handledUIDs())

				close(in.ch)
				waitForDone(t, done)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			in := &chanInput{ch: make(chan string, 16)}
			mgr.input = in
			tc.run(t, mgr, in, out)
		})
	}
}

// MUST-NOT-FLIP: invalid transaction UIDs are rejected at admission WITHOUT
// terminal output. The other no-terminal admission outcomes are already
// pinned by existing tests and are intentionally not duplicated here:
// duplicate ACTIVE UIDs by TestManager_FlowScenarios case "duplicate active
// uid is ignored without corrupting lane progression", and duplicate
// TOMBSTONED UIDs by case "duplicate tombstoned uid is ignored without extra
// terminal output" (both in manager_flow_test.go).
func TestCharacterization_InvalidInvocationAdmission(t *testing.T) {
	tests := map[string]struct {
		fn Function
	}{
		"empty transaction UID is ignored without terminal output": {
			fn: Function{Name: "fn"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newFlowManager()
			called := false
			mgr.Register("fn", func(Function) { called = true })

			mgr.dispatchInvocation(context.Background(), &tc.fn)

			assert.False(t, called, "invalid UID must be rejected at admission before handler execution")
			assert.NotContains(t, out.String(), "FUNCTION_RESULT_BEGIN")
		})
	}
}
