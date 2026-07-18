// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

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
	if err != nil {
		t.Fatal(err)
	}
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.FinishShutdown() })
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := jobmgr.NewCommandKernel(
		run, admission, uids, tasks, frames, clock,
		make(chan lifecycle.AdmissionGrant, 1), nil,
		jobmgr.RunShutdownBarrierFunc(func(context.Context, uint64) error { return nil }),
		jobmgr.RunFinalizerFunc(func(context.Context, uint64) error { return nil }),
		externalPlanner{}, catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	loop, err := jobmgr.NewKernelLoop(kernel)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	if err := loop.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := kernel.SubmitAndWait(context.Background(), jobmgr.Request{
		UID: "concrete-catalog", Source: lifecycle.SourceFunction, Route: "direct",
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN concrete-catalog 500 application/json ")) {
		t.Fatalf("concrete Function result differs: %q", output.Bytes())
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if census := catalog.Census(); !census.Closed || census.Routes != 0 ||
		census.InvocationLeases != 0 || census.PendingCleanups != 0 ||
		census.CompletedCleanups != 1 || cleanupCalls.Load() != 1 {
		t.Fatalf("kernel did not close concrete Function catalog: census=%+v cleanup=%d",
			census, cleanupCalls.Load())
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeExternalUIDLedger(t, uids)
}

func TestKernelGenericFunctionInvocationsOnSameRouteRunConcurrently(t *testing.T) {
	const calls = lifecycle.TransientTaskSlots + 1

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
					return lifecycle.NewControlResult(
						lifecycle.ControlInternal,
					)
				},
				Cleanup: func(context.Context) error {
					cleanupCalls.Add(1)
					return nil
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	kernel, run, admission, uids := newExternalKernel(t, catalog)
	for index := 0; index < calls; index++ {
		if err := kernel.Submit(context.Background(), jobmgr.Request{
			UID: fmt.Sprintf("same-route-%d", index), Source: lifecycle.SourceFunction,
			Route: "direct",
		}); err != nil {
			close(release)
			t.Fatal(err)
		}
	}

	seen := make(map[string]struct{}, calls)
	timer := time.NewTimer(time.Second)
	for len(seen) < lifecycle.TransientTaskSlots {
		select {
		case uid := <-entered:
			seen[uid] = struct{}{}
		case <-timer.C:
			close(release)
			kernel.Stop()
			_ = kernel.Wait(context.Background())
			t.Fatalf(
				"same-route handlers entered=%d, want %d concurrent entries",
				len(seen),
				lifecycle.TransientTaskSlots,
			)
		}
	}
	if !timer.Stop() {
		<-timer.C
	}
	select {
	case uid := <-entered:
		close(release)
		kernel.Stop()
		_ = kernel.Wait(context.Background())
		t.Fatalf(
			"handler %q bypassed the %d-slot execution bound",
			uid,
			lifecycle.TransientTaskSlots,
		)
	case <-time.After(25 * time.Millisecond):
	}
	if cleanupCalls.Load() != 0 {
		close(release)
		kernel.Stop()
		_ = kernel.Wait(context.Background())
		t.Fatal("handler generation cleaned while invocations were active")
	}

	close(release)
	select {
	case uid := <-entered:
		seen[uid] = struct{}{}
	case <-time.After(time.Second):
		kernel.Stop()
		_ = kernel.Wait(context.Background())
		t.Fatal("fifth same-route invocation did not run after a slot released")
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if census := catalog.Census(); census.InvocationLeases != 0 ||
		census.CompletedCleanups != 1 || cleanupCalls.Load() != 1 {
		t.Fatalf(
			"same-route invocation cleanup differs: census=%+v calls=%d",
			census,
			cleanupCalls.Load(),
		)
	}
	if len(seen) != calls {
		t.Fatalf("same-route handlers entered=%d, want %d", len(seen), calls)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
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
					return lifecycle.NewControlResult(
						lifecycle.ControlInternal,
					)
				},
			},
			CooperativeCancel: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	kernel, run, admission, uids := newExternalKernel(t, catalog)
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
			t.Fatalf("same-route handlers entered=%v, want both", seen)
		}
	}
	if err := kernel.Cancel(context.Background(), "cancelled"); err != nil {
		close(releaseBlocked)
		t.Fatal(err)
	}
	select {
	case err := <-results["cancelled"]:
		if err != nil {
			close(releaseBlocked)
			t.Fatalf("cancelled invocation terminal error: %v", err)
		}
	case <-time.After(time.Second):
		close(releaseBlocked)
		t.Fatal("cancelled invocation did not reach terminal")
	}
	select {
	case err := <-results["blocked"]:
		close(releaseBlocked)
		t.Fatalf(
			"cancelling one same-route invocation completed its peer: %v",
			err,
		)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseBlocked)
	select {
	case err := <-results["blocked"]:
		if err != nil {
			t.Fatalf("blocked invocation terminal error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked invocation did not complete after release")
	}

	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	kernel, run, admission, uids := newExternalKernel(t, catalog)

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
	if err != nil {
		t.Fatal(err)
	}
	if err := kernel.QuiesceFunctions(
		context.Background(),
		mutation,
	); err != nil {
		t.Fatal(err)
	}
	rejected, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "during-mutation", Route: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Rejected != lifecycle.ControlUnavailable ||
		catalog.Census().Version != 1 {
		t.Fatalf(
			"quiesced catalog decision=%+v census=%+v",
			rejected,
			catalog.Census(),
		)
	}
	version, err := kernel.CommitFunctions(context.Background(), mutation)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("mutation version=%d, want 2", version)
	}
	select {
	case <-oldCleaned:
	case <-time.After(time.Second):
		t.Fatal("retired handler cleanup did not run through TaskSupervisor")
	}
	if err := kernel.SubmitAndWait(context.Background(), jobmgr.Request{
		UID: "after-mutation", Source: lifecycle.SourceFunction, Route: "direct",
	}); err != nil {
		t.Fatal(err)
	}
	if oldCalls.Load() != 0 || newCalls.Load() != 1 {
		t.Fatalf("post-mutation dispatch differs: old=%d new=%d", oldCalls.Load(), newCalls.Load())
	}

	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-newCleaned:
	default:
		t.Fatal("replacement handler was not cleaned during catalog close")
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeExternalUIDLedger(t, uids)
}

func TestFunctionCatalogMutationCancellationAfterHandoffWaitsForDisposition(t *testing.T) {
	catalog := &handoffMutationCatalog{
		begun: make(chan struct{}),
		allow: make(chan struct{}),
	}
	kernel, run, admission, uids := newExternalKernel(t, catalog)
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
		version, err := kernel.CommitFunctions(
			context.Background(),
			mutation,
		)
		done <- mutationResult{version: version, err: err}
	}()
	select {
	case <-catalog.begun:
	case <-time.After(time.Second):
		t.Fatal("Function mutation did not reach catalog admission")
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
			t.Fatal("accepted Function mutation did not complete")
		}
	}

	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeExternalUIDLedger(t, uids)

	if early != nil {
		t.Fatalf(
			"accepted Function mutation returned before disposition: version=%d err=%v",
			result.version,
			result.err,
		)
	}
	if result.version != 2 || result.err != nil {
		t.Fatalf("Function mutation disposition=%+v, want committed version 2", result)
	}
}

func TestFunctionCatalogPausedMutationAbortsDuringShutdown(t *testing.T) {
	catalog := &handoffMutationCatalog{
		begun: make(chan struct{}),
		allow: make(chan struct{}),
	}
	close(catalog.allow)
	kernel, run, admission, uids := newExternalKernel(t, catalog)
	mutation := handoffMutation{}
	if err := kernel.QuiesceFunctions(
		context.Background(),
		mutation,
	); err != nil {
		t.Fatal(err)
	}
	if !catalog.active.Load() {
		t.Fatal("quiesced mutation did not remain catalog-owned")
	}

	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if catalog.active.Load() || !catalog.closed.Load() {
		t.Fatalf(
			"shutdown catalog state: active=%v closed=%v",
			catalog.active.Load(),
			catalog.closed.Load(),
		)
	}
	if _, err := kernel.CommitFunctions(
		context.Background(),
		mutation,
	); err == nil {
		t.Fatal("shutdown accepted a paused mutation commit")
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeExternalUIDLedger(t, uids)
}

type handoffMutation struct{}

func (handoffMutation) FunctionCatalogMutation() {}

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
	error,
) error {
	return nil
}

func (catalog *handoffMutationCatalog) BeginMutation(
	jobmgr.FunctionCatalogMutation,
) error {
	catalog.active.Store(true)
	close(catalog.begun)
	return nil
}

func (catalog *handoffMutationCatalog) AdvanceMutationQuiesce(
	int,
) (jobmgr.FunctionCatalogMutationProgress, error) {
	select {
	case <-catalog.allow:
		return jobmgr.FunctionCatalogMutationProgress{
			CompletedNodes: 1,
			TotalNodes:     1,
			Version:        1,
			Quiesced:       true,
		}, nil
	default:
		return jobmgr.FunctionCatalogMutationProgress{
			TotalNodes: 1,
			Version:    1,
		}, nil
	}
}

func (*handoffMutationCatalog) ResumeMutation(
	jobmgr.FunctionCatalogMutation,
) error {
	return nil
}

func (catalog *handoffMutationCatalog) AdvanceMutation(
	int,
	*[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan,
) (jobmgr.FunctionCatalogMutationProgress, int, error) {
	catalog.active.Store(false)
	return jobmgr.FunctionCatalogMutationProgress{
		CompletedNodes: 1,
		TotalNodes:     1,
		Version:        2,
		Done:           true,
	}, 0, nil
}

func (catalog *handoffMutationCatalog) AbortMutation(
	*[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan,
) (int, error) {
	catalog.active.Store(false)
	return 0, nil
}

func (catalog *handoffMutationCatalog) BeginClose() error {
	catalog.closed.Store(true)
	return nil
}

func (*handoffMutationCatalog) CloseStep(
	int,
	*[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan,
) (int, bool, error) {
	return 0, false, nil
}

func (catalog *handoffMutationCatalog) LifecycleCensus() jobmgr.FunctionCatalogCensus {
	return jobmgr.FunctionCatalogCensus{
		Version:        2,
		MutationActive: catalog.active.Load(),
		Closed:         catalog.closed.Load(),
	}
}

func newExternalKernel(t *testing.T, catalog jobmgr.FunctionCatalogPort) (*jobmgr.CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger) {
	t.Helper()
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.FinishShutdown() })
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := jobmgr.NewCommandKernel(
		run, admission, uids, tasks, frames, clock,
		make(chan lifecycle.AdmissionGrant, 1), nil,
		jobmgr.RunShutdownBarrierFunc(func(context.Context, uint64) error { return nil }),
		jobmgr.RunFinalizerFunc(func(context.Context, uint64) error { return nil }),
		externalPlanner{}, catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	loop, err := jobmgr.NewKernelLoop(kernel)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	if err := loop.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return kernel, run, admission, uids
}

type externalPlanner struct{}

func (externalPlanner) Plan(jobmgr.Request) (jobmgr.WorkPlan, error) {
	return jobmgr.WorkPlan{
		Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			return lifecycle.NewControlResult(lifecycle.ControlInternal)
		}),
	}, nil
}

func closeExternalUIDLedger(t *testing.T, ledger *lifecycle.UIDLedger) {
	t.Helper()
	for {
		more, err := ledger.CloseBatch(lifecycle.UIDReturnBatch)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
}
