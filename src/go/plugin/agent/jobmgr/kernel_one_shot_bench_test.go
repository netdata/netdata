// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

// Measures the real cleanup task launch, completion, termination and release.
// Bookkeeping is O(1) per task; timing is a local trend, not a CI threshold.
func BenchmarkBKernelFunctionCleanupLifecycle(b *testing.B) {
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, time.Second)
	require.NoError(b, err)
	b.Cleanup(func() { _ = run.FinishShutdown() })
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(b, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(b, err)
	var released, completed uint64
	work := func(context.Context) (lifecycle.TaskOutcome, error) {
		return lifecycle.NoValueOutcome(), nil
	}
	catalog := functionCatalogPortStub{
		release: func(FunctionInvocationRef) (FunctionCleanupPlan, error) {
			released++
			return NewFunctionCleanupPlan(FunctionCleanupRef(released), work)
		},
		complete: func(ref FunctionCleanupRef) error {
			completed++
			if uint64(ref) != completed || tasks.Active() != 0 {
				b.Fatal("cleanup acknowledged out of order or before task release")
			}
			return nil
		},
	}
	kernel, err := NewCommandKernel(run, lifecycle.NewUIDLedger(), tasks, frames, lifecycle.RealClock{},
		newNoopRunShutdownBarrier(), newNoopRunFinalizer(), catalog)
	require.NoError(b, err)
	require.NoError(b, run.OpenAdmission())
	b.ReportAllocs()
	for b.Loop() {
		if err := kernel.releaseFunctionInvocation(FunctionInvocationRef{
			Slot:       1,
			Generation: 1,
		}); err != nil {
			b.Fatal(err)
		}
		kernel.serviceFunctionCleanupBacklog(1)
		kernel.serviceTaskStarts(1)
		kernel.completeTask(<-tasks.CompletionCh())
		kernel.acknowledgeTask(<-tasks.AcknowledgementCh())
	}
	require.Equal(b, released, completed)
	require.NoError(b, run.DirtyCause())
}
