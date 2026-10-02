// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKernelOneShotInvalidCompletionSequenceAbandons(t *testing.T) {
	for _, kind := range []string{"barrier", "finalizer", "function-cleanup"} {
		t.Run(kind, func(t *testing.T) {
			for name, sequence := range map[string]uint8{"zero": 0, "unexpected phase": 2, "overflow": 255} {
				t.Run(name, func(t *testing.T) {
					fixture := startOneShotProtocolFixture(t, kind)
					t.Cleanup(func() {
						// Join the child if the regression prevents abandonment, before
						// the fixture's cleanup releases its supervisor slot.
						if !fixture.ackReceived {
							require.NoError(t, fixture.kernel.tasks.Abandon(fixture.completion.Ref, 2))
							fixture.receiveAcknowledgment(t)
						}
					})
					malformed := fixture.completion
					malformed.Sequence = sequence
					fixture.kernel.completeTask(malformed)
					require.Error(t, fixture.kernel.run.DirtyCause())
					require.Equal(t, 1, fixture.kernel.tasks.Active())
					fixture.assertCatalogCompletions(t, 0)

					ack := fixture.receiveAcknowledgment(t)
					require.Equal(t, lifecycle.TaskAcknowledgement{
						Ref:      fixture.completion.Ref,
						Sequence: 2,
						Kind:     lifecycle.TaskActionAbandon,
						Abandoned: lifecycle.TaskAbandonment{
							Outcome: lifecycle.TaskOutcomeNone,
						},
					}, ack)
					fixture.kernel.acknowledgeTask(ack)
					require.Zero(t, fixture.kernel.tasks.Active())
					fixture.assertCatalogCompletions(t, 1)
				})
			}
		})
	}
}

func TestKernelOneShotRejectsWrongAcknowledgmentAction(t *testing.T) {
	for _, kind := range []string{"barrier", "finalizer", "function-cleanup"} {
		t.Run(kind, func(t *testing.T) {
			fixture := startOneShotProtocolFixture(t, kind)
			fixture.kernel.completeTask(fixture.completion)
			ack := fixture.receiveAcknowledgment(t)
			require.Equal(t, lifecycle.TaskActionTerminate, ack.Kind)

			wrong := ack
			wrong.Kind = lifecycle.TaskActionAbandon
			fixture.kernel.acknowledgeTask(wrong)
			assert.Equal(t, 1, fixture.kernel.tasks.Active(), "wrong action must not release a joined task")
			assert.Zero(t, fixture.catalogCompletions, "wrong action must not acknowledge catalog cleanup")
			assert.Error(t, fixture.kernel.run.DirtyCause())

			fixture.kernel.acknowledgeTask(ack)
			assert.Zero(t, fixture.kernel.tasks.Active(), "the genuine acknowledgment must remain usable")
			fixture.assertCatalogCompletions(t, 1)
		})
	}
}

func TestKernelOneShotDuplicateCompletionPreservesAcceptedAction(t *testing.T) {
	for _, kind := range []string{"barrier", "finalizer", "function-cleanup"} {
		t.Run(kind, func(t *testing.T) {
			fixture := startOneShotProtocolFixture(t, kind)
			fixture.kernel.completeTask(fixture.completion)
			ack := fixture.receiveAcknowledgment(t)

			// Hold the real acknowledgment while replaying completion. The child is
			// joined, but the kernel still owns its accepted termination handshake.
			fixture.kernel.completeTask(fixture.completion)
			assert.Error(t, fixture.kernel.run.DirtyCause())
			assert.Equal(t, 1, fixture.kernel.tasks.Active())
			fixture.kernel.acknowledgeTask(ack)
			assert.Zero(t, fixture.kernel.tasks.Active(), "replay must not replace the accepted terminal action")
			fixture.assertCatalogCompletions(t, 1)
			if kind == "barrier" {
				assert.False(t, fixture.kernel.shutdownBarrierSettled())
			}
			if kind == "finalizer" {
				assert.False(t, fixture.kernel.runCensus().RunFinalizerComplete)
			}
		})
	}
}

func TestKernelOneShotDuplicateAcknowledgmentDoesNotReleaseTwice(t *testing.T) {
	for _, kind := range []string{"barrier", "finalizer", "function-cleanup"} {
		t.Run(kind, func(t *testing.T) {
			fixture := startOneShotProtocolFixture(t, kind)
			fixture.kernel.completeTask(fixture.completion)
			ack := fixture.receiveAcknowledgment(t)
			fixture.kernel.acknowledgeTask(ack)
			require.Zero(t, fixture.kernel.tasks.Active())
			fixture.assertCatalogCompletions(t, 1)

			fixture.kernel.acknowledgeTask(ack)
			assert.Zero(t, fixture.kernel.tasks.Active())
			fixture.assertCatalogCompletions(t, 1)
			assert.Error(t, fixture.kernel.run.DirtyCause())
		})
	}
}

func TestKernelOneShotReleaseFailureRetainsOwnership(t *testing.T) {
	for _, kind := range []string{"barrier", "finalizer", "function-cleanup"} {
		t.Run(kind, func(t *testing.T) {
			fixture := startOneShotProtocolFixture(t, kind)
			fixture.kernel.completeTask(fixture.completion)
			ack := fixture.receiveAcknowledgment(t)

			// Inject a real supervisor release guard after the child joins. One-shot
			// work does not normally retain timeouts; this tests failure ownership.
			_, err := fixture.kernel.tasks.MarkRetainedTimeout(ack.Ref)
			require.NoError(t, err)
			fixture.kernel.acknowledgeTask(ack)
			assert.Equal(t, 1, fixture.kernel.tasks.Active())
			assert.Zero(t, fixture.catalogCompletions)
			assert.Error(t, fixture.kernel.run.DirtyCause())

			_, err = fixture.kernel.tasks.ClearRetainedTimeout(ack.Ref)
			require.NoError(t, err)
			fixture.kernel.acknowledgeTask(ack)
			assert.Zero(t, fixture.kernel.tasks.Active(), "failed release must preserve the acknowledgment owner")
			fixture.assertCatalogCompletions(t, 1)
			if kind == "barrier" {
				assert.False(t, fixture.kernel.shutdownBarrierSettled())
			}
			if kind == "finalizer" {
				assert.False(t, fixture.kernel.runCensus().RunFinalizerComplete)
			}
		})
	}
}

type oneShotProtocolFixture struct {
	kernel             *testCommandKernel
	kind               string
	completion         lifecycle.TaskCompletion
	ackReceived        bool
	catalogCompletions int
}

func startOneShotProtocolFixture(t *testing.T, kind string) *oneShotProtocolFixture {
	t.Helper()
	fixture := &oneShotProtocolFixture{
		kind: kind,
	}
	const cleanupRef = FunctionCleanupRef(73)
	catalog := functionCatalogPortStub{
		release: func(FunctionInvocationRef) (FunctionCleanupPlan, error) {
			return NewFunctionCleanupPlan(cleanupRef, func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			})
		},
		complete: func(ref FunctionCleanupRef) error {
			assert.Equal(t, cleanupRef, ref, "catalog acknowledgment must preserve exact cleanup identity")
			fixture.catalogCompletions++
			return nil
		},
	}
	kernel, run, uids, tasks := newKernelWithClockFinalizerCatalogAndTimeout(
		t, stoppedKernelPlanner{}, catalog, io.Discard, lifecycle.RealClock{}, newNoopRunFinalizer(), time.Second,
	)
	fixture.kernel = kernel
	t.Cleanup(func() {
		// On a red assertion the child is still joined; release its supervisor
		// slot directly so an expected protocol failure cannot leak task state.
		if fixture.ackReceived && tasks.Active() != 0 {
			_, err := tasks.ClearRetainedTimeout(fixture.completion.Ref)
			assert.NoError(t, err)
			assert.NoError(t, tasks.Release(fixture.completion.Ref))
		}
		closeUIDLedger(t, uids)
	})
	require.NoError(t, run.OpenAdmission())
	if kind == "function-cleanup" {
		require.NoError(t, kernel.releaseFunctionInvocation(FunctionInvocationRef{
			Slot:       1,
			Generation: 1,
		}))
		kernel.serviceFunctionCleanupBacklog(1)
	} else {
		require.NoError(t, kernel.beginShutdown(time.Now().Add(time.Second)))
		require.NoError(t, kernel.advanceShutdownAuthority())
		if kind == "finalizer" {
			completeNoopShutdownBarrier(t, kernel)
			kernel.serviceFunctionCatalogClose(MaximumFunctionCloseQuantum)
			require.NoError(t, kernel.advanceRunFinalizer())
		} else {
			require.NoError(t, kernel.advanceShutdownBarrier())
		}
	}
	kernel.serviceTaskStarts(1)
	require.Equal(t, 1, tasks.Active())
	select {
	case fixture.completion = <-tasks.CompletionCh():
	case <-time.After(time.Second):
		require.FailNow(t, "one-shot work did not complete")
	}
	return fixture
}

func (fixture *oneShotProtocolFixture) receiveAcknowledgment(t *testing.T) lifecycle.TaskAcknowledgement {
	t.Helper()
	select {
	case ack := <-fixture.kernel.tasks.AcknowledgementCh():
		fixture.ackReceived = true
		return ack
	case <-time.After(time.Second):
		require.FailNow(t, "one-shot terminal action was not acknowledged")
		return lifecycle.TaskAcknowledgement{}
	}
}

func (fixture *oneShotProtocolFixture) assertCatalogCompletions(t *testing.T, want int) {
	t.Helper()
	if fixture.kind != "function-cleanup" {
		want = 0
	}
	assert.Equal(t, want, fixture.catalogCompletions)
}
