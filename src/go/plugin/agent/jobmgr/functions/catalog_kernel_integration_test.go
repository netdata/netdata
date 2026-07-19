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
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	clock := lifecycle.RealClock{}
	run, err := lifecycle.NewRunSupervisor(1, clock, 5*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
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
	require.NoError(t, err)
	loop, err := jobmgr.NewKernelLoop(kernel)
	require.NoError(t, err)

	require.NoError(t, run.OpenAdmission())

	require.NoError(t, loop.Start(context.Background()))

	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))

	census := catalog.Census()
	require.False(t, !census.Closed ||
		census.Routes != 0 ||
		census.InvocationLeases != 0 ||
		census.PendingCleanups != 0 ||
		census.CompletedCleanups != population ||
		cleanupCalls.Load() != population)

	published := catalog.storage.published.Load()
	require.EqualValues(t, 0, published)

	cleanup := catalog.storage.cleanup.Load()
	require.EqualValues(t, 0, cleanup)

	total := catalog.storage.total.Load()
	require.EqualValues(t, 0, total)

	require.False(t, catalog.storage.preparation.Load())
	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	require.NoError(t, admission.CloseDrained(run.Generation()))

	closeCleanupIntegrationUIDs(t, uids)
}

type cleanupIntegrationPlanner struct{}

func (cleanupIntegrationPlanner) Plan(
	jobmgr.Request,
) (jobmgr.WorkPlan, error) {
	return jobmgr.WorkPlan{
		Work: lifecycle.FrameTaskWork(
			func(context.Context) (lifecycle.SealedResult, error) {
				return lifecycle.NewControlResult(lifecycle.ControlInternal)
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
		require.NoError(t, err)
		if !more {
			return
		}
	}
}
