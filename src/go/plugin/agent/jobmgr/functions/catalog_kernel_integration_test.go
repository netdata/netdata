// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestFunctionCatalogCleanupBacklogDrainsThroughKernelLifecycle(
	t *testing.T,
) {
	const population = jobmgr.MaximumFunctionCleanupBatch

	declarations := testCleanupDeclarations(population)
	var cleanupCalls atomic.Int32
	for index := range declarations {
		declarations[index].Generation.Cleanup = func(context.Context) error {
			cleanupCalls.Add(1)
			return nil
		}
	}
	catalog, err := NewCatalog(declarations)
	if err != nil {
		t.Fatal(err)
	}
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, 5*time.Second)
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
		run,
		admission,
		uids,
		tasks,
		frames,
		clock,
		make(chan lifecycle.AdmissionGrant, 1),
		nil,
		jobmgr.RunShutdownBarrierFunc(
			func(context.Context, uint64) error { return nil },
		),
		jobmgr.RunFinalizerFunc(
			func(context.Context, uint64) error { return nil },
		),
		cleanupIntegrationPlanner{},
		catalog,
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
	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}

	census := catalog.Census()
	if !census.Closed ||
		census.Routes != 0 ||
		census.InvocationLeases != 0 ||
		census.PendingCleanups != 0 ||
		census.CompletedCleanups != population ||
		cleanupCalls.Load() != population {
		t.Fatalf(
			"kernel cleanup lifecycle differs: census=%+v calls=%d",
			census,
			cleanupCalls.Load(),
		)
	}
	if published := catalog.storage.published.Load(); published != 0 {
		t.Fatalf("closed catalog retained %d published bytes", published)
	}
	if cleanup := catalog.storage.cleanup.Load(); cleanup != 0 {
		t.Fatalf("closed catalog retained %d cleanup bytes", cleanup)
	}
	if total := catalog.storage.total.Load(); total != 0 {
		t.Fatalf("closed catalog retained %d total bytes", total)
	}
	if catalog.storage.preparation.Load() {
		t.Fatal("closed catalog retained mutation preparation")
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"kernel cleanup retained Tasks: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if census := tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("kernel cleanup retained long-lived ownership: %+v", census)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeCleanupIntegrationUIDs(t, uids)
}

type cleanupIntegrationPlanner struct{}

func (cleanupIntegrationPlanner) Plan(
	jobmgr.Request,
) (jobmgr.WorkPlan, error) {
	return jobmgr.WorkPlan{
		Work: lifecycle.FrameTaskWork(
			func(context.Context) (lifecycle.SealedResult, error) {
				return lifecycle.NewControlResult(
					lifecycle.ControlInternal,
				)
			},
		),
	}, nil
}

func closeCleanupIntegrationUIDs(
	t *testing.T,
	uids *lifecycle.UIDLedger,
) {
	t.Helper()
	for {
		more, err := uids.CloseBatch(lifecycle.UIDReturnBatch)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
}
