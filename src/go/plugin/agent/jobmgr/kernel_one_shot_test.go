// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func newOneShotKernel(
	t *testing.T,
	barrier RunShutdownBarrier,
	finalizer RunFinalizer,
	catalog FunctionCatalogPort,
) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.TaskSupervisor) {
	t.Helper()
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	kernel, err := NewCommandKernel(
		run,
		lifecycle.NewUIDLedger(),
		tasks,
		frames,
		lifecycle.RealClock{},
		barrier,
		finalizer,
		catalog,
	)
	require.NoError(t, err)
	require.NoError(t, run.OpenAdmission())
	return kernel, run, tasks
}

func TestKernelOneShotBarrierFailure(t *testing.T) {
	want := errors.New("barrier failed")
	for name, test := range map[string]struct {
		work func(context.Context, uint64) error
		want error
	}{
		"error": {work: func(context.Context, uint64) error { return want }, want: want},
		"panic": {work: func(context.Context, uint64) error { panic("barrier panic") }, want: lifecycle.ErrTaskPanic},
	} {
		t.Run(name, func(t *testing.T) {
			var barrierCalls, finalizerCalls int
			catalog := &oneShotClosingCatalog{}
			kernel, run, tasks := newOneShotKernel(t,
				runShutdownBarrierFunc(func(ctx context.Context, generation uint64) error {
					barrierCalls++
					return test.work(ctx, generation)
				}),
				runFinalizerFunc(func(context.Context, uint64) error { finalizerCalls++; return nil }), catalog)
			require.NoError(t, kernel.Start(context.Background()))
			kernel.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			require.ErrorIs(t, kernel.Wait(ctx), test.want)
			require.Equal(t, 1, barrierCalls)
			require.Zero(t, finalizerCalls)
			require.False(t, catalog.closed)
			require.Zero(t, tasks.Active())
			require.Zero(t, tasks.Pending())
			require.False(t, run.TerminalState().Quiescent)
		})
	}
}

type oneShotClosingCatalog struct {
	functionCatalogPortStub
	closed bool
	close  func()
}

func (catalog *oneShotClosingCatalog) BeginClose() error {
	catalog.closed = true
	if catalog.close != nil {
		catalog.close()
	}
	return nil
}

func (catalog *oneShotClosingCatalog) LifecycleDrained() bool { return catalog.closed }

func TestKernelOneShotShutdownOrdering(t *testing.T) {
	barrierEntered := make(chan struct{})
	barrierRelease := make(chan struct{})
	events := make(chan string, 4)
	catalog := &oneShotClosingCatalog{
		close: func() { events <- "catalog" },
	}
	kernel, run, tasks := newOneShotKernel(t,
		runShutdownBarrierFunc(func(ctx context.Context, _ uint64) error {
			close(barrierEntered)
			select {
			case <-barrierRelease:
				events <- "barrier"
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
		runFinalizerFunc(func(context.Context, uint64) error { events <- "finalizer"; return nil }), catalog)
	require.NoError(t, kernel.Start(context.Background()))
	kernel.Stop()
	select {
	case <-barrierEntered:
	case <-time.After(time.Second):
		t.Fatal("barrier did not start")
	}
	select {
	case event := <-events:
		t.Fatalf("shutdown advanced while barrier owned its work: %s", event)
	default:
	}
	close(barrierRelease)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, kernel.Wait(ctx))
	require.Equal(t, "barrier", <-events)
	require.Equal(t, "catalog", <-events)
	require.Equal(t, "finalizer", <-events)
	require.Empty(t, events)
	require.Zero(t, tasks.Active())
	require.True(t, run.TerminalState().Quiescent)
}

func receiveOneShotCompletion(t *testing.T, tasks *lifecycle.TaskSupervisor) lifecycle.TaskCompletion {
	t.Helper()
	select {
	case completion := <-tasks.CompletionCh():
		return completion
	case <-time.After(time.Second):
		t.Fatal("one-shot work did not complete")
		return lifecycle.TaskCompletion{}
	}
}

func receiveOneShotAcknowledgement(t *testing.T, tasks *lifecycle.TaskSupervisor) lifecycle.TaskAcknowledgement {
	t.Helper()
	select {
	case ack := <-tasks.AcknowledgementCh():
		return ack
	case <-time.After(time.Second):
		t.Fatal("one-shot action was not acknowledged")
		return lifecycle.TaskAcknowledgement{}
	}
}

func TestKernelOneShotFunctionCleanupDisposition(t *testing.T) {
	workErr := errors.New("cleanup work failed")
	catalogErr := errors.New("catalog acknowledgement failed")
	for name, test := range map[string]struct {
		work       lifecycle.TaskWork
		catalogErr error
		abandon    bool
	}{
		"work error":    {work: func(context.Context) (lifecycle.TaskOutcome, error) { return lifecycle.NoValueOutcome(), workErr }},
		"catalog error": {work: func(context.Context) (lifecycle.TaskOutcome, error) { return lifecycle.NoValueOutcome(), nil }, catalogErr: catalogErr},
		"unexpected outcome": {work: func(context.Context) (lifecycle.TaskOutcome, error) {
			result, err := lifecycle.NewControlResult(lifecycle.ControlInternal)
			if err != nil {
				return lifecycle.TaskOutcome{}, err
			}
			return lifecycle.NewFrameOutcome(result)
		}, abandon: true},
	} {
		t.Run(name, func(t *testing.T) {
			const cleanupRef FunctionCleanupRef = 37
			var tasks *lifecycle.TaskSupervisor
			var completed []FunctionCleanupRef
			catalog := functionCatalogPortStub{
				release: func(FunctionInvocationRef) (FunctionCleanupPlan, error) {
					return NewFunctionCleanupPlan(cleanupRef, test.work)
				},
				complete: func(ref FunctionCleanupRef) error {
					require.Zero(t, tasks.Active(), "catalog acknowledgement must follow physical task release")
					completed = append(completed, ref)
					return test.catalogErr
				},
			}
			kernel, run, supervisor := newOneShotKernel(t, newNoopRunShutdownBarrier(), newNoopRunFinalizer(), catalog)
			tasks = supervisor
			require.NoError(t, kernel.releaseFunctionInvocation(FunctionInvocationRef{
				Slot:       1,
				Generation: 1,
			}))
			kernel.serviceFunctionCleanupBacklog(1)
			kernel.serviceTaskStarts(1)
			completion := receiveOneShotCompletion(t, tasks)
			kernel.completeTask(completion)
			require.Empty(t, completed)
			if !test.abandon {
				require.NoError(t, run.DirtyCause(), "ordinary cleanup errors are reported after acknowledgment")
			}
			ack := receiveOneShotAcknowledgement(t, tasks)
			if test.abandon {
				require.Equal(t, lifecycle.TaskActionAbandon, ack.Kind)
			}
			kernel.acknowledgeTask(ack)
			require.Equal(t, []FunctionCleanupRef{cleanupRef}, completed)
			require.Empty(t, kernel.functionCleanupTasks)
			require.Zero(t, tasks.Active())
			require.Error(t, run.DirtyCause())
			if completion.Err != nil {
				require.ErrorIs(t, run.DirtyCause(), workErr)
			}
			if test.catalogErr != nil {
				require.ErrorIs(t, run.DirtyCause(), catalogErr)
			}
		})
	}
}
