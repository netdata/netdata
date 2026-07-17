// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"bytes"
	"context"
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
			ID: "method", PublicName: "direct", Lane: functionadapter.RouteLane(),
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
		PublicName: "direct", Lane: functionadapter.RouteLane(),
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
		PublicName: "direct", Lane: functionadapter.RouteLane(),
	}
	mutation, err := catalog.NewMutation(catalog.Census().Version, []functionadapter.RouteChange{{
		PublicName: "direct", Declaration: &replacement,
	}})
	if err != nil {
		t.Fatal(err)
	}
	version, err := kernel.MutateFunctions(context.Background(), mutation)
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
	go func() {
		version, err := kernel.MutateFunctions(ctx, handoffMutation{})
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

func (catalog *handoffMutationCatalog) AdvanceMutation(
	int,
	*[jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan,
) (jobmgr.FunctionCatalogMutationProgress, int, error) {
	select {
	case <-catalog.allow:
		catalog.active.Store(false)
		return jobmgr.FunctionCatalogMutationProgress{
			CompletedNodes: 1,
			TotalNodes:     1,
			Version:        2,
			Done:           true,
		}, 0, nil
	default:
		return jobmgr.FunctionCatalogMutationProgress{
			TotalNodes: 1,
			Version:    1,
		}, 0, nil
	}
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
	for batch := 0; batch < lifecycle.MaximumUIDRecords/lifecycle.UIDReturnBatch; batch++ {
		more, err := ledger.CloseBatch(lifecycle.UIDReturnBatch)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
	t.Fatal("UID close exceeded fixed batch bound")
}
