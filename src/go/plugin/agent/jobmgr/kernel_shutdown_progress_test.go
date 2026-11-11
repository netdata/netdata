// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestKernelAdvancesShutdownWhenRunBecomesDirtyDuringDispatch(t *testing.T) {
	clock := newKernelFinalizerClock()
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(
		t, stoppedKernelPlanner{}, io.Discard, clock, newNoopRunFinalizer(), time.Second,
	)
	dispatch := &dispatchObserverGate{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	require.NoError(t, tasks.BindRuntimeObserver(dispatch))
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(dispatch.release) }) }
	t.Cleanup(func() {
		release()
		kernel.Stop()
		kernel.NotifyControlReady()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = kernel.Wait(ctx)
	})
	require.NoError(t, run.OpenAdmission())
	require.NoError(t, kernel.Start(t.Context()))
	cancelled := make(chan struct{})
	require.NoError(t, kernel.SubmitPrepared(t.Context(), Request{
		UID:     "dirty-during-dispatch",
		LaneKey: "dirty-during-dispatch",
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/test",
	}, WorkPlan{
		Work: func(ctx context.Context) (lifecycle.TaskOutcome, error) {
			<-ctx.Done()
			close(cancelled)
			result, err := lifecycle.NewControlResult(lifecycle.ControlInternal)
			if err != nil {
				return lifecycle.TaskOutcome{}, err
			}
			return lifecycle.NewFrameOutcome(result)
		},
	}))
	select {
	case <-dispatch.entered:
	case <-time.After(time.Second):
		t.Fatal("task dispatch did not reach its gate")
	}
	// Model the submission notification having already woken the loop. No new
	// input should be needed to cancel the child and advance shutdown.
	select {
	case <-kernel.wake:
	default:
	}
	cause := errors.New("construction failed during dispatch")
	run.Dirty(cause)
	release()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the dispatched task")
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	require.ErrorIs(t, kernel.Wait(waitCtx), cause)
	terminal := run.TerminalState()
	require.True(t, terminal.Reached)
	require.True(t, terminal.Quiescent)
	require.ErrorIs(t, terminal.Dirty, cause)
	closeUIDLedger(t, uids)
}

// Pause dispatch without calling back into an owner from the metrics observer.
type dispatchObserverGate struct {
	kernelRuntimeObserver
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (gate *dispatchObserverGate) SetRuntimeGauge(gauge lifecycle.RuntimeGauge, value int) {
	if gauge == lifecycle.RuntimeGaugeTasksActive && value == 1 {
		gate.once.Do(func() {
			close(gate.entered)
			<-gate.release
		})
	}
}
