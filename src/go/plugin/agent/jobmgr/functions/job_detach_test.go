// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestFunctionControllerDetachDefersInvocationDrain(t *testing.T) {
	for _, cancelInvocation := range []bool{false, true} {
		name := "admitted invocation"
		if cancelInvocation {
			name = "physical callback after cancellation"
		}
		t.Run(name, func(t *testing.T) {
			handler := &detachBlockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
			controller, catalog, _ := newDetachController(t, map[string]funcapi.MethodHandler{"job": handler})
			handle := publishDetachJob(t, controller, "job")
			decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "held", Route: "module:method"})
			require.NoError(t, err)
			require.True(t, decision.Lease.Valid())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, callErr := decision.Plan.Work(ctx)
				done <- callErr
			}()
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(handler.release) }) }
			callReturned, leaseReleased := false, false
			t.Cleanup(func() {
				release()
				if !callReturned {
					require.NoError(t, <-done)
				}
				if !leaseReleased {
					releaseDetachInvocation(t, catalog, decision.Lease)
				}
				require.NoError(t, handle.CloseAndDrain(context.Background()))
			})
			select {
			case <-handler.entered:
			case <-time.After(time.Second):
				t.Fatal("Function did not enter its handler")
			}
			if cancelInvocation {
				cancel()
				require.NoError(t, <-done)
				callReturned = true
				releaseDetachInvocation(t, catalog, decision.Lease)
				leaseReleased = true
			}

			detachCtx, stopDetach := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer stopDetach()
			require.NoError(t, handle.Detach(detachCtx), "detachment must not wait for admitted calls")
			require.Zero(t, handler.cleanupCount())
			next, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "after-detach", Route: "module:method"})
			require.NoError(t, err)
			require.False(t, next.Lease.Valid())
			require.NotZero(t, next.Rejected)

			finalizeCtx, stopFinalize := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer stopFinalize()
			require.ErrorIs(t, handle.Finalize(finalizeCtx), context.DeadlineExceeded)
			require.Zero(t, handler.cleanupCount(), "physical callback still owns handler state")
			release()
			if !callReturned {
				require.NoError(t, <-done)
				callReturned = true
			}
			if !leaseReleased {
				releaseDetachInvocation(t, catalog, decision.Lease)
				leaseReleased = true
			}
			require.NoError(t, handle.Finalize(context.Background()))
			require.NoError(t, handle.Finalize(context.Background()))
			require.Equal(t, 1, handler.cleanupCount())
		})
	}
}

func TestFunctionControllerDetachRetainsOlderSharedGeneration(t *testing.T) {
	handlerA := &controllerTestHandler{}
	handlerB := &detachBlockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
	close(handlerB.release)
	controller, catalog, _ := newDetachController(t, map[string]funcapi.MethodHandler{"a": handlerA, "b": handlerB})
	handleA := publishDetachJob(t, controller, "a")
	old, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "old-shared", Route: "module:method"})
	require.NoError(t, err)
	require.True(t, old.Lease.Valid())
	released := false
	t.Cleanup(func() {
		if !released {
			releaseDetachInvocation(t, catalog, old.Lease)
		}
		require.NoError(t, handleA.CloseAndDrain(context.Background()))
	})
	handleB := publishDetachJob(t, controller, "b")
	t.Cleanup(func() { require.NoError(t, handleB.CloseAndDrain(context.Background())) })
	require.NoError(t, handleA.Detach(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, handleA.Finalize(ctx), context.DeadlineExceeded)
	require.Zero(t, handlerA.cleanupCount())
	current, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "sibling", Route: "module:method", Args: []string{"__job:b"}})
	require.NoError(t, err)
	require.True(t, current.Lease.Valid())
	_, err = current.Plan.Work(context.Background())
	require.NoError(t, err)
	select {
	case <-handlerB.entered:
	default:
		t.Fatal("remaining sibling was not invoked")
	}
	releaseDetachInvocation(t, catalog, current.Lease)
	releaseDetachInvocation(t, catalog, old.Lease)
	released = true
	require.NoError(t, handleA.Finalize(context.Background()))
	require.Equal(t, 1, handlerA.cleanupCount())
	require.Zero(t, handlerB.cleanupCount())
}

func TestFunctionControllerDetachPreservesWithdrawalFailure(t *testing.T) {
	handler := &controllerTestHandler{}
	controller, catalog, wire := newDetachController(t, map[string]funcapi.MethodHandler{"job": handler})
	handle := publishDetachJob(t, controller, "job")
	withdrawErr := errors.New("withdraw failed")
	wire.withdrawErr = withdrawErr
	require.ErrorIs(t, handle.Detach(context.Background()), withdrawErr)
	require.False(t, handle.detached)
	require.True(t, handle.published)
	require.ErrorIs(t, handle.Finalize(context.Background()), withdrawErr)
	require.Zero(t, handler.cleanupCount())
	// The controller's existing shutdown path owns settlement after a dirty
	// publication failure; the handle must not clean resources speculatively.
	wire.withdrawErr = nil
	require.ErrorIs(t, controller.BeginShutdown(1), withdrawErr)
	require.NoError(t, catalog.BeginClose())
	for {
		cleanups, more, err := catalog.CloseStep(jobmgr.MaximumFunctionCloseQuantum)
		require.NoError(t, err)
		for _, cleanup := range cleanups {
			runCleanupPlan(t, catalog, cleanup)
		}
		if !more {
			break
		}
	}
	require.NoError(t, handle.Finalize(context.Background()))
	require.Equal(t, 1, handler.cleanupCount())
}

func newDetachController(t *testing.T, handlers map[string]funcapi.MethodHandler) (*Controller, *Catalog, *recordingPublicationPort) {
	t.Helper()
	controller, catalog, err := newContainedControllerTest(t, 1, collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "method"}} },
			MethodHandler:   func(job collectorapi.RuntimeJob) funcapi.MethodHandler { return handlers[job.Name()] },
		},
	})
	require.NoError(t, err)
	wire := newRecordingPublicationPort()
	publication, err := NewPublication(1, wire)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&controllerTestMutationPort{catalog: catalog}, publication))
	require.NoError(t, controller.Activate())
	return controller, catalog, wire
}

func publishDetachJob(t *testing.T, controller *Controller, name string) *JobHandle {
	t.Helper()
	job := &controllerTestJob{fullName: "module_" + name, module: "module", name: name, running: true}
	handle, err := prepareControllerTestJob(t, controller, lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}, job)
	require.NoError(t, err)
	require.NoError(t, handle.Publish())
	return handle
}

func releaseDetachInvocation(t *testing.T, catalog *Catalog, lease jobmgr.FunctionInvocationRef) {
	t.Helper()
	cleanup, err := catalog.ReleaseInvocation(lease)
	require.NoError(t, err)
	if cleanup.Valid() {
		runCleanupPlan(t, catalog, cleanup)
	}
}

type detachBlockingHandler struct {
	controllerTestHandler
	entered chan struct{}
	release chan struct{}
}

func (h *detachBlockingHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	close(h.entered)
	<-h.release
	return &funcapi.FunctionResponse{Status: 200}
}
