// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

type runShutdownBarrierFunc func(context.Context, uint64) error

func (fn runShutdownBarrierFunc) BeforeFunctionCatalogClose(
	ctx context.Context,
	generation uint64,
) error {
	return fn(ctx, generation)
}

type runFinalizerFunc func(context.Context, uint64) error

func (fn runFinalizerFunc) FinalizeRun(ctx context.Context, generation uint64) error {
	return fn(ctx, generation)
}

func TestFunctionCatalogKernelIntegration(t *testing.T) {
	var cleanupCalls atomic.Int32
	catalog, err := functionadapter.NewCatalog([]functionadapter.Declaration{
		{
			ID: "method", PublicName: "direct",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: "external-test",
				Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				},
				Cleanup: func(context.Context) error {
					cleanupCalls.Add(1)
					return nil
				},
			},
		},
	})
	require.NoError(t, err)
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	uids := lifecycle.NewUIDLedger()
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	kernel, err := jobmgr.NewCommandKernel(
		run, uids, tasks, frames, clock,
		runShutdownBarrierFunc(func(context.Context, uint64) error { return nil }),
		runFinalizerFunc(func(context.Context, uint64) error { return nil }),
		catalog,
	)
	require.NoError(t, err)

	require.NoError(t, run.OpenAdmission())
	require.NoError(t, kernel.Start(context.Background()))

	require.NoError(t, kernel.SubmitAndWait(context.Background(), jobmgr.Request{
		UID: "concrete-catalog", Source: lifecycle.SourceFunction, Route: "direct",
	}),
	)

	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN concrete-catalog 500 application/json ")))
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	census := catalog.Census()
	require.False(t, !census.Closed || census.Routes != 0 ||
		census.InvocationLeases != 0 || census.PendingCleanups != 0 ||
		cleanupCalls.Load() != 1)

	closeExternalUIDLedger(t, uids)
}

func TestKernelGenericFunctionInvocationsOnSameRouteRunConcurrently(t *testing.T) {
	const calls = 9

	entered := make(chan string, calls)
	release := make(chan struct{})
	var cleanupCalls atomic.Int32
	catalog, err := functionadapter.NewCatalog([]functionadapter.Declaration{
		{
			ID:         "method",
			PublicName: "direct",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: "concurrent-external-test",
				Handler: func(
					_ context.Context,
					input functionadapter.HandlerInput,
				) (lifecycle.SealedResult, error) {
					entered <- input.UID
					<-release
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				},
				Cleanup: func(context.Context) error {
					cleanupCalls.Add(1)
					return nil
				},
			},
		},
	})
	require.NoError(t, err)
	kernel, _, uids := newExternalKernel(t, catalog)
	for index := range calls {
		if err := kernel.Submit(context.Background(), jobmgr.Request{
			UID: fmt.Sprintf("same-route-%d", index), Source: lifecycle.SourceFunction,
			Route: "direct",
		}); err != nil {
			close(release)
			require.FailNow(t, "test failed", err)
		}
	}

	seen := make(map[string]struct{}, calls)
	timer := time.NewTimer(time.Second)
	for len(seen) < calls {
		select {
		case uid := <-entered:
			seen[uid] = struct{}{}
		case <-timer.C:
			close(release)
			kernel.Stop()
			_ = kernel.Wait(context.Background())
			require.FailNowf(t, "test failed", "same-route handlers entered=%d, want %d concurrent entries", len(seen), calls)
		}
	}
	if !timer.Stop() {
		<-timer.C
	}
	if cleanupCalls.Load() != 0 {
		close(release)
		kernel.Stop()
		_ = kernel.Wait(context.Background())
		require.FailNow(t, "test failed", "handler generation cleaned while invocations were active")
	}

	close(release)
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	census := catalog.Census()
	require.False(t, census.InvocationLeases != 0 || cleanupCalls.Load() != 1)

	require.EqualValues(t, calls, len(seen))

	closeExternalUIDLedger(t, uids)
}

func TestKernelSameRouteFunctionCancellationIsInvocationLocal(t *testing.T) {
	entered := make(chan string, 2)
	releaseBlocked := make(chan struct{})
	catalog, err := functionadapter.NewCatalog([]functionadapter.Declaration{
		{
			ID:         "method",
			PublicName: "direct",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: "cancellation-external-test",
				Handler: func(
					ctx context.Context,
					input functionadapter.HandlerInput,
				) (lifecycle.SealedResult, error) {
					entered <- input.UID
					if input.UID == "blocked" {
						<-releaseBlocked
					} else {
						<-ctx.Done()
					}
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				},
			},
			CooperativeCancel: true,
		},
	})
	require.NoError(t, err)
	kernel, _, uids := newExternalKernel(t, catalog)
	results := map[string]chan error{
		"blocked":   make(chan error, 1),
		"cancelled": make(chan error, 1),
	}
	for uid, result := range results {
		go func() {
			result <- kernel.SubmitAndWait(
				context.Background(),
				jobmgr.Request{
					UID: uid, Source: lifecycle.SourceFunction,
					Route: "direct",
				},
			)
		}()
	}
	seen := make(map[string]struct{}, len(results))
	for len(seen) < len(results) {
		select {
		case uid := <-entered:
			seen[uid] = struct{}{}
		case <-time.After(time.Second):
			close(releaseBlocked)
			kernel.Stop()
			_ = kernel.Wait(context.Background())
			require.FailNowf(t, "test failed", "same-route handlers entered=%v, want both", seen)
		}
	}
	if err := kernel.Cancel(context.Background(), "cancelled"); err != nil {
		close(releaseBlocked)
		require.FailNow(t, "test failed", err)
	}
	select {
	case err := <-results["cancelled"]:
		if err != nil {
			close(releaseBlocked)
			require.FailNowf(t, "test failed", "cancelled invocation terminal error: %v", err)
		}
	case <-time.After(time.Second):
		close(releaseBlocked)
		require.FailNow(t, "test failed", "cancelled invocation did not reach terminal")
	}
	select {
	case err := <-results["blocked"]:
		close(releaseBlocked)
		require.FailNowf(t, "test failed", "cancelling one same-route invocation completed its peer: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseBlocked)
	select {
	case err := <-results["blocked"]:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "blocked invocation did not complete after release")
	}

	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	closeExternalUIDLedger(t, uids)
}

func TestFunctionCatalogMutationUsesKernelLoop(t *testing.T) {
	var oldCalls atomic.Int32
	var newCalls atomic.Int32
	oldCleaned := make(chan struct{}, 1)
	newCleaned := make(chan struct{}, 1)
	oldGeneration := &functionadapter.HandlerGenerationDeclaration{
		ID: "old",
		Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
			oldCalls.Add(1)
			return lifecycle.NewControlResult(lifecycle.ControlInternal)
		},
		Cleanup: func(context.Context) error {
			oldCleaned <- struct{}{}
			return nil
		},
	}
	catalog, err := functionadapter.NewCatalog([]functionadapter.Declaration{{
		ID: "method", Generation: oldGeneration,
		PublicName: "direct",
	}})
	require.NoError(t, err)
	kernel, _, uids := newExternalKernel(t, catalog)

	newGeneration := &functionadapter.HandlerGenerationDeclaration{
		ID: "new",
		Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
			newCalls.Add(1)
			return lifecycle.NewControlResult(lifecycle.ControlInternal)
		},
		Cleanup: func(context.Context) error {
			newCleaned <- struct{}{}
			return nil
		},
	}
	replacement := functionadapter.Declaration{
		ID: "method", Generation: newGeneration,
		PublicName: "direct",
	}
	mutation, err := catalog.NewMutation(catalog.Census().Version, []functionadapter.RouteChange{{
		PublicName: "direct", Declaration: &replacement,
	}})
	require.NoError(t, err)

	require.NoError(t, kernel.QuiesceFunctions(context.Background(), mutation))

	rejected, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "during-mutation", Route: "direct",
	})
	require.NoError(t, err)
	require.False(t, rejected.Rejected != lifecycle.ControlUnavailable || catalog.Census().Version != 1)
	version, err := kernel.CommitFunctions(context.Background(), mutation)
	require.NoError(t, err)
	require.EqualValues(t, 2, version)
	select {
	case <-oldCleaned:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "retired handler cleanup did not run through TaskSupervisor")
	}

	require.NoError(t, kernel.SubmitAndWait(context.Background(), jobmgr.Request{
		UID: "after-mutation", Source: lifecycle.SourceFunction, Route: "direct",
	}),
	)

	require.False(t, oldCalls.Load() != 0 || newCalls.Load() != 1)

	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	select {
	case <-newCleaned:
	default:
		require.FailNow(t, "test failed", "replacement handler was not cleaned during catalog close")
	}

	closeExternalUIDLedger(t, uids)
}

func TestFunctionCatalogShutdownCompletesResumedMultiTurnMutation(t *testing.T) {
	const population = 2*jobmgr.MaximumFunctionMutationQuantum + 1

	var predecessorCleanups atomic.Int32
	var successorCleanups atomic.Int32
	declarations := make([]functionadapter.Declaration, population)
	replacements := make([]functionadapter.Declaration, population)
	changes := make([]functionadapter.RouteChange, population)
	for index := range population {
		name := fmt.Sprintf("multi-turn-%03d", index)
		declarations[index] = functionadapter.Declaration{
			ID:         "method",
			PublicName: name,
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: fmt.Sprintf("predecessor-%03d", index),
				Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				},
				Cleanup: func(context.Context) error {
					predecessorCleanups.Add(1)
					return nil
				},
			},
		}
		replacements[index] = functionadapter.Declaration{
			ID:         "method",
			PublicName: name,
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: fmt.Sprintf("successor-%03d", index),
				Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				},
				Cleanup: func(context.Context) error {
					successorCleanups.Add(1)
					return nil
				},
			},
		}
		changes[index] = functionadapter.RouteChange{
			PublicName:  name,
			Declaration: &replacements[index],
		}
	}
	catalog, err := functionadapter.NewCatalog(declarations)
	require.NoError(t, err)
	firstCommitStep := make(chan struct{})
	resumeCommit := make(chan struct{})
	observed := &observedFunctionCatalog{
		FunctionCatalogPort: catalog,
		firstCommitStep:     firstCommitStep,
		resumeCommit:        resumeCommit,
	}
	kernel, _, uids := newExternalKernel(t, observed)
	mutation, err := catalog.NewMutation(catalog.Census().Version, changes)
	require.NoError(t, err)
	require.NoError(t, kernel.QuiesceFunctions(context.Background(), mutation))

	type commitResult struct {
		version uint64
		err     error
	}
	committed := make(chan commitResult, 1)
	go func() {
		version, err := kernel.CommitFunctions(context.Background(), mutation)
		committed <- commitResult{version: version, err: err}
	}()
	select {
	case <-firstCommitStep:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "multi-turn mutation did not complete its first commit step")
	}
	kernel.Stop()
	close(resumeCommit)

	select {
	case result := <-committed:
		require.NoError(t, result.err)
		require.EqualValues(t, 2, result.version)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "test failed", "resumed mutation did not finish during shutdown")
	}
	require.NoError(t, kernel.Wait(context.Background()))

	census := catalog.Census()
	require.True(t, census.Closed)
	require.Zero(t, census.Routes)
	require.Zero(t, census.PendingCleanups)
	require.EqualValues(t, population, predecessorCleanups.Load())
	require.EqualValues(t, population, successorCleanups.Load())
	closeExternalUIDLedger(t, uids)
}

func TestFunctionCatalogMutationCancellationAfterHandoffWaitsForDisposition(t *testing.T) {
	catalog := &handoffMutationCatalog{
		begun: make(chan struct{}),
		allow: make(chan struct{}),
	}
	kernel, _, uids := newExternalKernel(t, catalog)
	ctx, cancel := context.WithCancel(context.Background())
	type mutationResult struct {
		version uint64
		err     error
	}
	done := make(chan mutationResult, 1)
	mutation := handoffMutation{}
	go func() {
		err := kernel.QuiesceFunctions(ctx, mutation)
		if err != nil {
			done <- mutationResult{err: err}
			return
		}
		version, err := kernel.CommitFunctions(context.Background(), mutation)
		done <- mutationResult{version: version, err: err}
	}()
	select {
	case <-catalog.begun:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Function mutation did not reach catalog admission")
	}
	cancel()

	var early *mutationResult
	select {
	case result := <-done:
		early = &result
	case <-time.After(100 * time.Millisecond):
	}
	close(catalog.allow)
	kernel.NotifyControlReady()
	var result mutationResult
	if early != nil {
		result = *early
	} else {
		select {
		case result = <-done:
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "accepted Function mutation did not complete")
		}
	}

	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	closeExternalUIDLedger(t, uids)

	require.Nil(t, early)
	require.False(t, result.version != 2 || result.err != nil)
}

func TestFunctionCatalogPausedMutationAbortsDuringShutdown(t *testing.T) {
	catalog := &handoffMutationCatalog{
		begun: make(chan struct{}),
		allow: make(chan struct{}),
	}
	close(catalog.allow)
	kernel, run, uids := newExternalKernel(t, catalog)
	mutation := handoffMutation{}

	require.NoError(t, kernel.QuiesceFunctions(context.Background(), mutation))

	require.True(t, catalog.active.Load())

	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.False(t, catalog.active.Load() || !catalog.closed.Load())

	_, err := kernel.CommitFunctions(context.Background(), mutation)
	stopping, ok :=
		errors.AsType[*lifecycle.StoppingRejection](err)
	require.True(t, ok)
	require.EqualValues(
		t,
		run.Generation(),
		stopping.Generation,
	)

	closeExternalUIDLedger(t, uids)
}

func TestFunctionCatalogMutationAfterShutdownCutIsRejectedBeforeAdmission(
	t *testing.T,
) {
	catalog := &handoffMutationCatalog{
		begun: make(chan struct{}),
		allow: make(chan struct{}),
	}
	kernel, run, uids := newExternalKernel(t, catalog)

	kernel.Stop()
	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancelShutdown()
	require.NoError(t, kernel.WaitShutdownStarted(shutdownCtx))

	err := kernel.QuiesceFunctions(
		context.Background(),
		handoffMutation{},
	)
	stopping, ok :=
		errors.AsType[*lifecycle.StoppingRejection](err)
	require.True(t, ok)
	require.EqualValues(t, run.Generation(), stopping.Generation)
	require.False(t, catalog.active.Load())
	select {
	case <-catalog.begun:
		require.FailNow(
			t,
			"test failed",
			"post-cut Function mutation reached catalog admission",
		)
	default:
	}

	require.NoError(t, kernel.Wait(context.Background()))
	require.True(t, catalog.closed.Load())
	closeExternalUIDLedger(t, uids)
}

type handoffMutation struct{}

func (handoffMutation) FunctionCatalogMutation() {}

type observedFunctionCatalog struct {
	jobmgr.FunctionCatalogPort
	firstCommitStep chan struct{}
	resumeCommit    <-chan struct{}
	once            sync.Once
}

func (ofc *observedFunctionCatalog) AdvanceMutation(
	quantum int,
) (jobmgr.FunctionCatalogMutationProgress, []jobmgr.FunctionCleanupPlan, error) {
	progress, cleanups, err := ofc.FunctionCatalogPort.AdvanceMutation(quantum)
	if err == nil && !progress.Done {
		ofc.once.Do(func() {
			close(ofc.firstCommitStep)
			<-ofc.resumeCommit
		})
	}
	return progress, cleanups, err
}

type handoffMutationCatalog struct {
	begun  chan struct{}
	allow  chan struct{}
	active atomic.Bool
	closed atomic.Bool
}

func (*handoffMutationCatalog) ResolveAndAcquire(
	jobmgr.FunctionLookup,
) (jobmgr.FunctionCatalogDecision, error) {
	return jobmgr.FunctionCatalogDecision{}, nil
}

func (*handoffMutationCatalog) ReleaseInvocation(
	jobmgr.FunctionInvocationRef,
) (jobmgr.FunctionCleanupPlan, error) {
	return jobmgr.FunctionCleanupPlan{}, nil
}

func (*handoffMutationCatalog) CompleteCleanup(
	jobmgr.FunctionCleanupRef,
) error {
	return nil
}

func (hmc *handoffMutationCatalog) BeginMutation(
	jobmgr.FunctionCatalogMutation,
) error {
	hmc.active.Store(true)
	close(hmc.begun)
	return nil
}

func (hmc *handoffMutationCatalog) AdvanceMutationQuiesce(
	int,
) (jobmgr.FunctionCatalogMutationProgress, error) {
	select {
	case <-hmc.allow:
		return jobmgr.FunctionCatalogMutationProgress{
			Version:  1,
			Quiesced: true,
		}, nil
	default:
		return jobmgr.FunctionCatalogMutationProgress{
			Version: 1,
		}, nil
	}
}

func (*handoffMutationCatalog) ResumeMutation(
	jobmgr.FunctionCatalogMutation,
) error {
	return nil
}

func (hmc *handoffMutationCatalog) AdvanceMutation(
	int,
) (jobmgr.FunctionCatalogMutationProgress, []jobmgr.FunctionCleanupPlan, error) {
	hmc.active.Store(false)
	return jobmgr.FunctionCatalogMutationProgress{
		Version: 2,
		Done:    true,
	}, nil, nil
}

func (hmc *handoffMutationCatalog) AbortMutation(jobmgr.FunctionCatalogMutation) error {
	hmc.active.Store(false)
	return nil
}

func (hmc *handoffMutationCatalog) BeginClose() error {
	hmc.closed.Store(true)
	return nil
}

func (*handoffMutationCatalog) CloseStep(
	int,
) ([]jobmgr.FunctionCleanupPlan, bool, error) {
	return nil, false, nil
}

func (hmc *handoffMutationCatalog) LifecycleCensus() jobmgr.FunctionCatalogCensus {
	return jobmgr.FunctionCatalogCensus{
		Version:        2,
		MutationActive: hmc.active.Load(),
		Closed:         hmc.closed.Load(),
	}
}

func newExternalKernel(t *testing.T, catalog jobmgr.FunctionCatalogPort) (*jobmgr.CommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger) {
	t.Helper()
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	uids := lifecycle.NewUIDLedger()
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	kernel, err := jobmgr.NewCommandKernel(
		run, uids, tasks, frames, clock,
		runShutdownBarrierFunc(func(context.Context, uint64) error { return nil }),
		runFinalizerFunc(func(context.Context, uint64) error { return nil }),
		catalog,
	)
	require.NoError(t, err)

	require.NoError(t, run.OpenAdmission())
	require.NoError(t, kernel.Start(context.Background()))

	return kernel, run, uids
}

func closeExternalUIDLedger(t *testing.T, ledger *lifecycle.UIDLedger) {
	t.Helper()
	for {
		more, err := ledger.CloseBatch(lifecycle.UIDReturnBatch)
		require.NoError(t, err)
		if !more {
			return
		}
	}
}
