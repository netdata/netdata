// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestKernelCompletionBroadcastsToAllCallers(t *testing.T) {
	t.Run("repeat wait", func(t *testing.T) {
		kernel := newStoppedKernel(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		require.NoError(t, kernel.Wait(ctx))
	})

	t.Run("submit after stop", func(t *testing.T) {
		kernel := newStoppedKernel(t)
		catalog := testFunctionCatalogFor(t, kernel)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := kernel.Submit(ctx, Request{UID: "after-stop", Source: lifecycle.SourceFunction, Route: "route"})
		stopping, ok := errors.AsType[*lifecycle.StoppingRejection](err)
		require.True(t, ok)
		require.EqualValues(t, 1, stopping.Generation)
		require.False(t, catalog.next != 0 || catalog.release != 0 || len(catalog.leases) != 0)
	})

	t.Run("cancel after stop", func(t *testing.T) {
		kernel := newStoppedKernel(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := kernel.Cancel(ctx, "after-stop")
		stopping, ok := errors.AsType[*lifecycle.StoppingRejection](err)
		require.True(t, ok)
		require.EqualValues(t, 1, stopping.Generation)
	})
}

func TestKernelStopCutRejectsCancellationBeforeShutdownCompletes(
	t *testing.T,
) {
	kernel, run := newKernel(t)
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)

	kernel.Stop()

	err := kernel.Cancel(
		context.Background(),
		"after-stop-cut",
	)
	stopping, ok :=
		errors.AsType[*lifecycle.StoppingRejection](err)
	require.True(t, ok)
	require.EqualValues(
		t,
		run.Generation(),
		stopping.Generation,
	)
	require.NoError(t, kernel.Wait(context.Background()))
}

func TestCommandKernelStartsExactlyOnce(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T)
	}{
		"nil command kernel": {
			run: func(t *testing.T) {
				var kernel *CommandKernel
				require.Error(t, kernel.Start(context.Background()))
			},
		},
		"nil context": {
			run: func(t *testing.T) {
				kernel, _ := newKernel(t)
				require.Error(t, kernel.Start(nil))
			},
		},
		"duplicate start": {
			run: func(t *testing.T) {
				kernel, _ := newKernel(t)
				require.NoError(t, kernel.Start(context.Background()))
				require.Error(t, kernel.Start(context.Background()))

				kernel.Stop()

				require.NoError(t, kernel.Wait(context.Background()))
			},
		},
	}

	for name, test := range tests {
		t.Run(name, test.run)
	}
}

func TestKernelTerminalRejectsWithoutRetainingSubmissions(t *testing.T) {
	tests := map[string]struct {
		source lifecycle.Source
		call   func(context.Context, *testCommandKernel, int) error
	}{
		"command": {
			source: lifecycle.SourceJobManager,
			call: func(ctx context.Context, kernel *testCommandKernel, index int) error {
				return kernel.Submit(ctx, Request{
					UID:     fmt.Sprintf("terminal-command-%d", index),
					LaneKey: "lane",
					Source:  lifecycle.SourceJobManager,
					Route:   "route",
				})
			},
		},
		"control": {
			source: lifecycle.SourceFunction,
			call: func(ctx context.Context, kernel *testCommandKernel, index int) error {
				return kernel.Reject(ctx, fmt.Sprintf("terminal-control-%d", index), lifecycle.ControlBadRequest)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel := newStoppedKernel(t)
			for index := range externalSourceQueueDepth * 4 {
				err := test.call(context.Background(), kernel, index)
				stopping, ok := errors.AsType[*lifecycle.StoppingRejection](err)
				require.True(t, ok)
				require.EqualValues(t, 1, stopping.Generation)
			}

			retained := len(kernel.submissions[sourceIndex(test.source)])
			require.EqualValues(t, 0, retained)
		})
	}
}

func TestKernelDirtyStateTriggersFailStop(t *testing.T) {
	kernel, run := newKernel(t)
	startKernelLoop(t, kernel)
	want := errors.New("invariant failed")
	run.Dirty(want)
	kernel.NotifyControlReady()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.ErrorIs(t, kernel.Wait(ctx), want)

	require.ErrorIs(t, kernel.Wait(ctx), want)
}

func TestShutdownProbeObservesKernelLoopCancellation(t *testing.T) {
	kernel, run, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probe, err := startShutdownProbe(ctx, kernel.CommandKernel, "shutdown-probe")
	require.NoError(t, err)
	select {
	case <-probe.cancelled:
		require.FailNow(t, "test failed", "probe cancelled before shutdown")
	default:
	}

	kernel.Stop()
	require.NoError(t, probe.waitCancellation(ctx))
	require.NoError(t, kernel.Wait(ctx))
	require.NoError(t, probe.waitSettlement(ctx))
	closeUIDLedger(t, uids)
}

func TestKernelResourcePublicationRunsOffLoop(t *testing.T) {
	publishRelease := make(chan struct{})
	resource := newKernelTestReadyResource("resource", publishRelease, nil)
	kernel, run, uids, tasks := newKernelWithPlanner(t, kernelResourcePlanner(t, resource, nil, nil))

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), time.Second)
	defer cancelProbe()
	probe, err := startShutdownProbe(
		probeCtx,
		kernel.CommandKernel,
		"publish-off-loop-shutdown-probe",
	)
	require.NoError(t, err)

	require.NoError(t, kernel.Submit(context.Background(), Request{
		UID: "publish-off-loop", LaneKey: "resource", Source: lifecycle.SourceJobManager, Route: "install",
	}),
	)

	select {
	case <-resource.publishEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "resource publication did not start")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	shutdownErr := probe.waitCancellation(ctx)
	cancel()
	close(publishRelease)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))
	require.NoError(t, probe.waitSettlement(waitCtx))

	require.NoError(t, shutdownErr)
	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelPlanningRunsOutsideLoop(t *testing.T) {
	planEntered := make(chan struct{})
	planRelease := make(chan struct{})
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		close(planEntered)
		<-planRelease
		return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	kernel, run, uids, _ := newKernelWithPlanner(t, planner)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), time.Second)
	defer cancelProbe()
	probe, err := startShutdownProbe(
		probeCtx,
		kernel.CommandKernel,
		"planning-off-loop-shutdown-probe",
	)
	require.NoError(t, err)
	submitResult := make(chan error, 1)
	go func() {
		submitResult <- kernel.Submit(context.Background(), Request{
			UID: "blocked-planning", LaneKey: "lane", Source: lifecycle.SourceJobManager, Route: "route",
		})
	}()
	select {
	case <-planEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "planning did not start")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	shutdownErr := probe.waitCancellation(ctx)
	cancel()
	close(planRelease)
	select {
	case <-submitResult:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "blocked planning caller did not return")
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))
	require.NoError(t, probe.waitSettlement(waitCtx))

	require.NoError(t, shutdownErr)

	closeUIDLedger(t, uids)
}

func TestFunctionCatalogPlanningWaitsForKernelLoop(t *testing.T) {
	planned := make(chan struct{}, 1)
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		planned <- struct{}{}
		return WorkPlan{
			Work: frameTaskWork(plannerPlanWork),
		}, nil
	})
	kernel, run, uids, _ := newKernelWithPlanner(t, planner)
	catalog := testFunctionCatalogFor(t, kernel)

	require.NoError(t, run.OpenAdmission())

	submitted := make(chan error, 1)
	go func() {
		submitted <- kernel.Submit(context.Background(), Request{
			UID:    "function-loop-planning",
			Source: lifecycle.SourceFunction, Route: "route",
		})
	}()

	calledBeforeStart := false
	select {
	case <-planned:
		calledBeforeStart = true
	case <-time.After(25 * time.Millisecond):
	}

	startKernelLoop(t, kernel)
	select {
	case err := <-submitted:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Function submission did not complete")
	}
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	closeUIDLedger(t, uids)
	require.False(t, calledBeforeStart)
	require.False(t, catalog.next != 1 || catalog.release != 1 || len(catalog.leases) != 0)
}

func TestFunctionCatalogDecisionOwnsExactLease(t *testing.T) {
	work := WorkPlan{Work: frameTaskWork(plannerPlanWork)}
	tests := map[string]struct {
		decision     FunctionCatalogDecision
		wantErr      bool
		wantReleases int
		wantStatus   string
	}{
		"resolved terminal": {
			decision: FunctionCatalogDecision{
				Plan:  work,
				Lease: FunctionInvocationRef{Slot: 1, Generation: 1},
			},
			wantReleases: 1,
			wantStatus:   " 200 ",
		},
		"closed rejection owns no lease": {
			decision:   FunctionCatalogDecision{Rejected: lifecycle.ControlNotFound},
			wantStatus: " 404 ",
		},
		"invalid resolved decision releases returned lease": {
			decision: FunctionCatalogDecision{
				ResourceID: strings.Repeat("r", maximumRequestMetadataBytes+1),
				Plan:       work,
				Lease:      FunctionInvocationRef{Slot: 1, Generation: 1},
			},
			wantErr:      true,
			wantReleases: 1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var resolves int
			var releases int
			catalog := functionCatalogPortStub{
				resolve: func(FunctionLookup) (FunctionCatalogDecision, error) {
					resolves++
					return test.decision, nil
				},
				release: func(ref FunctionInvocationRef) (FunctionCleanupPlan, error) {
					releases++
					if ref != test.decision.Lease {
						return FunctionCleanupPlan{}, errors.New("released Function lease differs")
					}
					return FunctionCleanupPlan{}, nil
				},
			}
			var output bytes.Buffer
			planner := stoppedKernelPlanner{}
			kernel, run, uids, _ := newKernelWithClockFinalizerCatalogAndTimeout(
				t, planner, catalog, &output, lifecycle.RealClock{}, newNoopRunFinalizer(), time.Second,
			)

			require.NoError(t, run.OpenAdmission())

			startKernelLoop(t, kernel)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			err := kernel.submitAndWait(ctx, Request{
				UID: "catalog-decision", Source: lifecycle.SourceFunction, Route: "route",
			})

			gotErr := err != nil
			require.EqualValues(t, test.wantErr, gotErr)

			kernel.Stop()

			require.NoError(t, kernel.Wait(context.Background()))

			require.False(t, resolves != 1 || releases != test.wantReleases)
			require.False(t, test.wantStatus != "" && !bytes.Contains(output.Bytes(), []byte(test.wantStatus)))

			closeUIDLedger(t, uids)
		})
	}
}

func TestFunctionHandlerCleanupRunsOffKernelLoop(t *testing.T) {
	cleanupCompleted := make(chan error, 1)
	var kernel *testCommandKernel
	catalog := functionCatalogPortStub{
		resolve: func(FunctionLookup) (FunctionCatalogDecision, error) {
			return FunctionCatalogDecision{
				Plan:  WorkPlan{Work: frameTaskWork(plannerPlanWork)},
				Lease: FunctionInvocationRef{Slot: 1, Generation: 1},
			}, nil
		},
		release: func(FunctionInvocationRef) (FunctionCleanupPlan, error) {
			return FunctionCleanupPlan{
				Ref: FunctionCleanupRef(1),
				Work: func(ctx context.Context) (lifecycle.TaskOutcome, error) {
					if err := kernel.Cancel(ctx, "cleanup-loop-barrier"); err != nil {
						return lifecycle.NoValueOutcome(), err
					}
					return lifecycle.NoValueOutcome(), nil
				},
			}, nil
		},
		complete: func(FunctionCleanupRef) error {
			cleanupCompleted <- nil
			return nil
		},
	}
	planner := stoppedKernelPlanner{}
	var run *lifecycle.RunSupervisor
	var uids *lifecycle.UIDLedger
	kernel, run, uids, _ = newKernelWithClockFinalizerCatalogAndTimeout(
		t, planner, catalog, io.Discard, lifecycle.RealClock{}, newNoopRunFinalizer(), time.Second,
	)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.submitAndWait(context.Background(), Request{
		UID: "cleanup-off-loop", Source: lifecycle.SourceFunction, Route: "route",
	}),
	)

	select {
	case err := <-cleanupCompleted:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Function cleanup did not complete through TaskSupervisor")
	}
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.False(t, len(kernel.functionCleanupRequests) != 0 || len(kernel.functionCleanupTasks) != 0)

	closeUIDLedger(t, uids)
}

func TestWorkPlanRejectsOversizedClaimKey(t *testing.T) {
	plan := WorkPlan{
		Work:   frameTaskWork(plannerPlanWork),
		Claims: []string{strings.Repeat("c", maximumClaimKeyBytes+1)},
	}
	require.Error(t, plan.validate())
}

func TestWorkPlanAcceptsClaimsBeyondFormerAggregateByteLimit(t *testing.T) {
	plan := WorkPlan{
		Work:   frameTaskWork(plannerPlanWork),
		Claims: make([]string, 5),
	}
	for index := range plan.Claims {
		plan.Claims[index] = strings.Repeat(string(rune('a'+index)), maximumClaimKeyBytes)
	}
	require.NoError(t, plan.validate())
}

func TestKernelTerminalNoResponseDisposalCompletesUIDOwnership(t *testing.T) {
	kernel, run, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	require.NoError(t, run.OpenAdmission())
	now := kernel.clock.Now()
	const uid = "terminal-no-response"
	require.NoError(t, uids.Admit(uid, now))
	request := Request{
		UID: uid, LaneKey: "internal", Source: lifecycle.SourceJobManager,
	}
	laneKey := commandLaneKey{key: request.LaneKey, source: request.Source}
	lane, err := kernel.allocateLane(laneKey, request)
	require.NoError(t, err)
	generation, err := lifecycle.NewOperation(
		1,
		uid,
		request.Source,
		request.LaneKey,
		false,
	)
	require.NoError(t, err)
	operation := &commandOperation{
		OperationGeneration: generation,
		request:             request,
		lane:                lane,
		deadline:            deadlineEntry{index: -1},
		runtimeStarted:      now,
	}
	lane.owners = 1
	lane.head = operation
	lane.tail = operation
	lane.active = operation
	kernel.operations[uid] = operation
	kernel.appendOperation(operation)
	kernel.appendRuntimeOperation(operation)

	operation.PoisonResponse()
	kernel.tryDispose(operation)

	require.NotContains(t, kernel.operations, uid)
	active, tombstones, closed := uids.Census()
	require.Zero(t, active)
	require.Zero(t, tombstones)
	require.False(t, closed)
	closeUIDLedger(t, uids)
}

func TestKernelRunCensusCountsOnlyActiveUIDOwnership(t *testing.T) {
	kernel, _, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	now := kernel.clock.Now()
	const uid = "run-census"
	require.NoError(t, uids.Admit(uid, now))

	require.EqualValues(t, 1, kernel.runCensus().UIDActive)

	require.NoError(t, uids.Complete(uid, true, now))
	require.Zero(t, kernel.runCensus().UIDActive)
	active, tombstones, _ := uids.Census()
	require.Zero(t, active)
	require.EqualValues(t, 1, tombstones)

	closeUIDLedger(t, uids)
}

func TestKernelResourceStopCompletesAfterOperationDeadline(t *testing.T) {
	stopRelease := make(chan struct{})
	resource := newKernelTestReadyResource("resource", nil, stopRelease)
	kernel, run, uids, tasks := newKernelWithPlanner(
		t,
		kernelResourcePlanner(t, resource, nil, nil),
	)
	observer := &kernelRuntimeObserver{}
	require.NoError(t, kernel.BindRuntimeObserver(observer))
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)
	require.NoError(t, kernel.submitAndWait(
		context.Background(),
		Request{
			UID: "install-before-deadline-stop", LaneKey: "resource",
			Source: lifecycle.SourceJobManager, Route: "install",
		},
	))

	result := make(chan error, 1)
	go func() {
		result <- kernel.submitAndWait(
			context.Background(),
			Request{
				UID: "deadline-stop", LaneKey: "resource",
				Source: lifecycle.SourceJobManager, Route: "stop",
				Deadline: time.Now().Add(250 * time.Millisecond),
			},
		)
	}()
	select {
	case <-resource.stopEntered:
	case err := <-result:
		require.FailNowf(
			t,
			"test failed",
			"resource stop reached terminal before starting: %v",
			err,
		)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "resource stop did not start")
	}

	timeout := time.Now().Add(time.Second)
	for observer.operationTimeouts.Load() != 1 {
		if time.Now().After(timeout) {
			close(stopRelease)
			require.FailNow(
				t,
				"test failed",
				"resource stop deadline was not observed",
			)
		}
		runtime.Gosched()
	}
	close(stopRelease)

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"resource stop did not finish after its deadline",
		)
	}
	require.EqualValues(t, 0, tasks.Active())

	kernel.Stop()
	require.NoError(t, kernel.Wait(context.Background()))
	closeUIDLedger(t, uids)
}

type kernelRuntimeObserver struct {
	operationTimeouts  atomic.Uint64
	operationsAdmitted atomic.Uint64
}

func (*kernelRuntimeObserver) SetRuntimeGauge(
	lifecycle.RuntimeGauge,
	int,
) {
}

func (*kernelRuntimeObserver) AddRuntimeGauge(
	lifecycle.RuntimeGauge,
	int,
) {
}

func (kro *kernelRuntimeObserver) AddRuntimeCounter(
	counter lifecycle.RuntimeCounter,
	delta uint64,
) {
	if counter == lifecycle.RuntimeCounterOperationTimeouts {
		kro.operationTimeouts.Add(delta)
	}
	if counter == lifecycle.RuntimeCounterOperationsAdmitted {
		kro.operationsAdmitted.Add(delta)
	}
}

func (*kernelRuntimeObserver) SetRuntimeTimestamp(
	lifecycle.RuntimeTimestamp,
	time.Time,
) {
}

func TestKernelRunsResourceTransactionInOriginalOperation(t *testing.T) {
	tests := map[string]struct {
		prepareErr       error
		wantSuccessor    bool
		wantEvents       []string
		wantResponseText string
	}{
		"replacement commits response and successor": {
			wantSuccessor:    true,
			wantEvents:       []string{"prepare", "apply", "cleanup"},
			wantResponseText: "replace-success",
		},
		"preparation failure restores exact current": {
			prepareErr:       errors.New("validation rejected"),
			wantEvents:       []string{"prepare"},
			wantResponseText: "replace-rejected",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var events []string
			current := newKernelTestReadyResource("resource", nil, nil)
			successor := newKernelTestReadyResource("resource", nil, nil)
			permitPlan := lifecycle.NewJobLongLivedPlan()
			planner := kernelTestTransactionPlanner{
				permitPlan: permitPlan,
				current:    current,
				successor:  successor,
				prepareErr: test.prepareErr,
				events:     &events,
			}
			var output bytes.Buffer
			kernel, run, uids, tasks :=
				newKernelWithPlannerAndWriter(t, planner, &output)

			require.NoError(t, run.OpenAdmission())

			startKernelLoop(t, kernel)

			require.NoError(t, kernel.submitAndWait(
				context.Background(),
				Request{
					UID:     "install-current",
					LaneKey: "resource",
					Source:  lifecycle.SourceJobManager,
					Route:   "install",
				},
			),
			)

			originalIdentity := current.identity
			uid := "replace-rejected"
			if test.wantSuccessor {
				uid = "replace-success"
			}

			err := kernel.submitAndWait(
				context.Background(),
				Request{
					UID:     uid,
					LaneKey: "resource",
					Source:  lifecycle.SourceJobManager,
					Route:   "replace",
				},
			)
			if test.prepareErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.prepareErr)
			}

			lane := kernel.lanes[resourceCommandLaneKey("resource")]
			require.NotNil(t, lane)
			if test.wantSuccessor {
				require.False(t, lane.current != successor ||
					lane.currentIdentity != successor.identity ||
					lane.currentIdentity == originalIdentity)
			} else {
				require.False(t, lane.current != current || lane.currentIdentity != originalIdentity)
			}
			require.False(t, lane.currentStopping || lane.retiringIdentity.Valid() || lane.transactionPlanned != 0)
			require.Equal(t, test.wantEvents, events)
			require.Contains(t, output.String(), test.wantResponseText)

			kernel.Stop()
			waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			require.NoError(t, kernel.Wait(waitCtx))

			require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

			closeUIDLedger(t, uids)
		})
	}
}

func TestKernelSharesResourceAuthorityAcrossSchedulingSources(t *testing.T) {
	var events []string
	current := newKernelTestReadyResource("resource", nil, nil)
	successor := newKernelTestReadyResource("resource", nil, nil)
	permitPlan := lifecycle.NewJobLongLivedPlan()
	resourcePlanner := kernelTestTransactionPlanner{
		permitPlan: permitPlan,
		current:    current,
		successor:  successor,
		events:     &events,
	}
	planner := plannerFunc(func(
		_ context.Context,
		route string,
		_ []string,
	) (WorkPlan, error) {
		return resourcePlanner.Plan(Request{
			Route:   route,
			LaneKey: "resource",
		})
	})
	var output bytes.Buffer
	kernel, run, uids, tasks :=
		newKernelWithPlannerAndWriter(t, planner, &output)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "resource"
	})

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.submitAndWait(
		context.Background(),
		Request{
			UID:     "install-from-job-manager",
			LaneKey: "resource",
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		},
	),
	)

	require.NoError(t, kernel.submitAndWait(
		context.Background(),
		Request{
			UID:    "replace-from-function",
			Source: lifecycle.SourceFunction,
			Route:  "replace",
		},
	),
	)

	lane := kernel.lanes[resourceCommandLaneKey("resource")]
	require.False(t, lane == nil ||
		lane.current != successor ||
		lane.currentIdentity != successor.identity ||
		lane.resourceSource != lifecycle.SourceFunction)
	resourceLanes := 0
	for key := range kernel.lanes {
		if key.resource && key.key == "resource" {
			resourceLanes++
		}
	}
	require.EqualValues(t, 1, resourceLanes)

	want := []string{"prepare", "apply", "cleanup"}
	require.Equal(t, want, events)

	require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN replace-from-function 200 ")

	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(waitCtx))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelDisposesCancelledPreparedResourceTransaction(t *testing.T) {
	var events []string
	current := newKernelTestReadyResource("resource", nil, nil)
	successor := newKernelTestReadyResource("resource", nil, nil)
	permitPlan := lifecycle.NewJobLongLivedPlan()
	planner := kernelTestTransactionPlanner{
		permitPlan:          permitPlan,
		current:             current,
		successor:           successor,
		waitForCancellation: true,
		events:              &events,
	}
	var output bytes.Buffer
	kernel, run, uids, tasks :=
		newKernelWithPlannerAndWriter(t, planner, &output)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.submitAndWait(
		context.Background(),
		Request{
			UID:     "install-before-cancel",
			LaneKey: "resource",
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		},
	),
	)

	originalIdentity := current.identity

	require.NoError(t, kernel.submitAndWait(
		context.Background(),
		Request{
			UID:      "replace-deadline",
			LaneKey:  "resource",
			Source:   lifecycle.SourceJobManager,
			Route:    "replace",
			Deadline: time.Now().Add(10 * time.Millisecond),
		},
	),
	)

	lane := kernel.lanes[resourceCommandLaneKey("resource")]
	require.False(t, lane == nil ||
		lane.current != current ||
		lane.currentIdentity != originalIdentity ||
		lane.currentStopping ||
		lane.transactionPlanned != 0)

	want := []string{"prepare", "dispose"}
	require.Equal(t, want, events)

	require.Contains(t, output.String(), "replace-deadline")

	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(waitCtx))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelPreparedInternalTransactionAppliesWithoutResponse(t *testing.T) {
	var events []string
	current := newKernelTestReadyResource("resource", nil, nil)
	successor := newKernelTestReadyResource("resource", nil, nil)
	permitPlan := lifecycle.NewJobLongLivedPlan()
	planner := kernelTestTransactionPlanner{
		permitPlan: permitPlan,
		current:    current,
		successor:  successor,
		events:     &events,
	}
	var output bytes.Buffer
	kernel, run, uids, tasks :=
		newKernelWithPlannerAndWriter(t, planner, &output)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.submitAndWait(
		context.Background(),
		Request{
			UID:     "prepared-install",
			LaneKey: "resource",
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		},
	),
	)

	request := Request{
		UID:     "prepared-replace",
		LaneKey: "resource",
		Source:  lifecycle.SourceJobManager,
		Route:   "typed/reconcile",
	}
	plan, err := planner.Plan(Request{
		LaneKey: request.LaneKey,
		Route:   "replace",
	})
	require.NoError(t, err)
	plan.NoResponse = true

	require.NoError(t, kernel.SubmitPreparedAndWait(context.Background(), request, plan))

	lane := kernel.lanes[resourceCommandLaneKey("resource")]
	require.False(t, lane == nil ||
		lane.current != successor ||
		lane.currentIdentity != successor.identity ||
		lane.currentStopping ||
		lane.transactionPlanned != 0)

	want := []string{"prepare", "apply", "cleanup"}
	require.Equal(t, want, events)

	require.NotContains(t, output.String(), request.UID)

	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelShutdownStopsResourceAfterActiveUserDrains(t *testing.T) {
	stopRelease := make(chan struct{})
	workEntered := make(chan struct{})
	workRelease := make(chan struct{})
	resource := newKernelTestReadyResource("resource", nil, stopRelease)
	kernel, run, uids, tasks := newKernelWithPlanner(t, kernelResourcePlanner(t, resource, workEntered, workRelease))
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "resource"
	})

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.submitAndWait(context.Background(), Request{
		UID: "install-before-use", LaneKey: "resource", Source: lifecycle.SourceJobManager, Route: "install",
	}),
	)

	useResult := make(chan error, 1)
	go func() {
		useResult <- kernel.submitAndWait(context.Background(), Request{
			UID: "active-resource-user", Source: lifecycle.SourceFunction, Route: "use",
		})
	}()
	select {
	case <-workEntered:
	case err := <-useResult:
		require.FailNowf(t, "test failed", "resource user reached terminal before starting: %v", err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "resource user did not start")
	}
	kernel.Stop()
	var overlap bool
	select {
	case <-resource.stopEntered:
		overlap = true
	case <-time.After(50 * time.Millisecond):
	}
	close(workRelease)
	select {
	case <-resource.stopEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "resource stop did not begin after its user drained")
	}
	close(stopRelease)
	select {
	case <-useResult:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "resource user did not reach terminal disposal")
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))

	require.False(t, overlap)
	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelShutdownTracksDynamicTaskPopulation(t *testing.T) {
	const population = 9

	stopRelease := make(chan struct{})
	resources := make(map[string]*kernelTestReadyResource, population)
	for index := range population {
		id := fmt.Sprintf("resource-%02d", index)
		resources[id] = newKernelTestReadyResource(id, nil, stopRelease)
	}
	permitPlan := lifecycle.NewJobLongLivedPlan()
	kernel, run, uids, tasks := newKernelWithPlanner(t, kernelTestResourceSetPlanner{
		permitPlan: permitPlan,
		resources:  resources,
	})

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	for id := range resources {
		require.NoError(t, kernel.submitAndWait(context.Background(), Request{
			UID:     "install-" + id,
			LaneKey: id,
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		}),
		)
	}

	kernel.Stop()
	for id, resource := range resources {
		select {
		case <-resource.stopEntered:
		case <-time.After(time.Second):
			require.FailNowf(t, "test failed", "resource %q did not begin shutdown", id)
		}
	}

	active := tasks.Active()
	require.EqualValues(t, population, active)

	require.EqualValues(t, population, tasks.LongLivedCensus().Active)

	close(stopRelease)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	closeUIDLedger(t, uids)
}

func TestKernelLoopContinuesPendingTaskStartsAcrossServiceQuanta(t *testing.T) {
	const genericPopulation = lifecycle.TaskStartServiceQuantum * 2

	kernel, run, uids, tasks := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)

	require.NoError(t, run.OpenAdmission())

	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		kernel.Stop()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		_ = kernel.Wait(waitCtx)
	})
	genericEntered := make(chan struct{}, genericPopulation)
	enqueueCleanup := func(slot uint32) {
		t.Helper()
		request, err := tasks.Enqueue(
			lifecycle.TaskClassGenericFunction,
			lifecycle.TaskPlan{
				Source: lifecycle.SourceFunction,
				Work: func(context.Context) (lifecycle.TaskOutcome, error) {
					genericEntered <- struct{}{}
					<-release
					return lifecycle.NoValueOutcome(), nil
				},
			},
		)
		require.NoError(t, err)
		kernel.functionCleanupRequests[request] = FunctionCleanupRef(slot)
	}
	for index := range genericPopulation {
		enqueueCleanup(uint32(index + 1))
	}

	startKernelLoop(t, kernel)
	for index := range genericPopulation {
		select {
		case <-genericEntered:
		case <-time.After(time.Second):
			require.FailNowf(t, "test failed", "generic task %d remained pending after a service quantum", index+1)
		}
	}

	controlEntered := make(chan struct{}, 1)

	require.NoError(t, kernel.SubmitPrepared(
		context.Background(),
		Request{
			UID:     "continued-framework-control",
			LaneKey: "framework-control",
			Source:  lifecycle.SourceJobManager,
			Route:   "control",
		},
		WorkPlan{
			Work: frameTaskWork(
				func(context.Context) (lifecycle.SealedResult, error) {
					controlEntered <- struct{}{}
					<-release
					return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
				},
			),
		},
	),
	)

	select {
	case <-controlEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "newly runnable framework-control task did not start")
	}

	releaseOnce.Do(func() { close(release) })
	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelAsyncEventServiceQuantumIsPhaseBalancedAndBounded(
	t *testing.T,
) {
	const population = asyncEventServiceQuantum + 1

	kernel := newStoppedKernel(t)
	kernel.cancel = make(chan string, population)
	for index := range population {
		kernel.cancel <- fmt.Sprintf("unknown-cancel-%02d", index)
	}

	count := kernel.serviceAsyncEvents(asyncEventServiceQuantum)
	require.EqualValues(t, asyncEventServiceQuantum, count)

	require.EqualValues(t, 1, len(kernel.cancel))

	count = kernel.serviceAsyncEvents(asyncEventServiceQuantum)
	require.EqualValues(t, 1, count)

	require.EqualValues(t, 0, len(kernel.cancel))
}

func TestKernelShutdownCancelsInitialOperationSweepBeforePendingTaskDispatch(
	t *testing.T,
) {
	const population = lifecycle.TaskStartServiceQuantum * 2

	kernel, run, uids, tasks := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)

	require.NoError(t, run.OpenAdmission())

	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		kernel.Stop()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		_ = kernel.Wait(waitCtx)
	})
	entered := make(chan string, population)
	for index := range population {
		uid := fmt.Sprintf("shutdown-fence-%02d", index)
		plan := WorkPlan{
			NoResponse: true,
			Work: frameTaskWork(
				func(context.Context) (lifecycle.SealedResult, error) {
					entered <- uid
					<-release
					return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
				},
			),
		}

		require.NoError(t, kernel.admit(
			Request{
				UID: uid, LaneKey: uid,
				Source: lifecycle.SourceJobManager,
			},
			plan,
		),
		)
	}
	for kernel.scheduleTasks(lifecycle.TaskStartServiceQuantum) {
	}
	require.EqualValues(t, population, tasks.Pending())

	kernel.Stop()
	startKernelLoop(t, kernel)
	for index := range lifecycle.TaskStartServiceQuantum {
		select {
		case <-entered:
		case <-time.After(time.Second):
			require.FailNowf(t, "test failed", "initial task %d did not start", index+1)
		}
	}
	select {
	case uid := <-entered:
		require.FailNowf(t, "test failed", "pending operation %q started after shutdown began", uid)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

func TestKernelRunFinalizerUsesSharedBudgetExactlyOnce(t *testing.T) {
	called := make(chan struct {
		generation uint64
		deadline   time.Time
	}, 1)
	finalizer := runFinalizerFunc(func(ctx context.Context, generation uint64) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("finalizer context has no deadline")
		}
		called <- struct {
			generation uint64
			deadline   time.Time
		}{generation: generation, deadline: deadline}
		return nil
	})
	kernel, run, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)

	require.NoError(t, run.OpenAdmission())

	budget, err := run.BeginShutdown()
	require.NoError(t, err)
	startKernelLoop(t, kernel)
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	call := <-called
	require.False(t, call.generation != run.Generation() || !call.deadline.Equal(budget.Deadline()))
	select {
	case duplicate := <-called:
		require.FailNowf(t, "test failed", "finalizer ran more than once: %+v", duplicate)
	default:
	}

	terminal := run.TerminalState()
	require.False(t, !terminal.Reached || !terminal.Quiescent || terminal.Dirty != nil)

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)
}

func TestKernelShutdownDeadlineWinsFinalizerCompletion(t *testing.T) {
	const helperEnv = "NETDATA_JOBMGR_FINALIZER_DEADLINE_HELPER"
	if os.Getenv(helperEnv) != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.Command(executable, "-test.run=^TestKernelShutdownDeadlineWinsFinalizerCompletion$")
		cmd.Env = append(os.Environ(), helperEnv+"=1")

		combinedOutput, combinedOutputErr := cmd.CombinedOutput()
		require.NoError(t, combinedOutputErr, string(combinedOutput))

		return
	}
	clock := newKernelFinalizerClock()
	started := make(chan struct{})
	finalizer := runFinalizerFunc(func(ctx context.Context, _ uint64) error {
		close(started)
		<-ctx.Done()
		return nil
	})
	kernel, run, _, _ := newKernelWithClockFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, clock, finalizer, time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "finalizer did not start before shutdown expiry")
	}
	clock.expireShutdown(t)

	err := kernel.Wait(context.Background())
	require.False(t, err == nil || !strings.Contains(err.Error(), "shutdown deadline exceeded"))

	terminal := run.TerminalState()
	require.False(t, !terminal.Reached || terminal.Quiescent || terminal.Dirty == nil)
}

func TestKernelDueClockWinsIndependentFinalizerCompletion(t *testing.T) {
	const helperEnv = "NETDATA_JOBMGR_FINALIZER_DUE_CLOCK_HELPER"
	if os.Getenv(helperEnv) != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.Command(executable, "-test.run=^TestKernelDueClockWinsIndependentFinalizerCompletion$")
		cmd.Env = append(os.Environ(), helperEnv+"=1")

		combinedOutput, combinedOutputErr := cmd.CombinedOutput()
		require.NoError(t, combinedOutputErr, string(combinedOutput))

		return
	}
	clock := newKernelFinalizerClock()
	started := make(chan struct{})
	release := make(chan struct{})
	finalizer := runFinalizerFunc(func(context.Context, uint64) error {
		close(started)
		<-release
		return nil
	})
	kernel, run, _, _ := newKernelWithClockFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, clock, finalizer, time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "finalizer did not start before shutdown expiry")
	}
	clock.advanceShutdownWithoutSignal(t)
	close(release)

	err := kernel.Wait(context.Background())
	require.False(t, err == nil || !strings.Contains(err.Error(), "shutdown deadline exceeded"))

	terminal := run.TerminalState()
	require.False(t, !terminal.Reached || terminal.Quiescent || terminal.Dirty == nil)
}

func TestKernelKeepsUnchangedDeadlineTimerAcrossUnrelatedEvents(t *testing.T) {
	clock := newKernelFinalizerClock()
	workEntered := make(chan struct{})
	releaseWork := make(chan struct{})
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: frameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
			close(workEntered)
			<-releaseWork
			return plannerPlanWork(ctx)
		})}, nil
	})
	kernel, run, uids, _ := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, newNoopRunFinalizer(), time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	done := make(chan error, 1)
	go func() {
		done <- kernel.submitAndWait(context.Background(), Request{
			UID: "one-deadline-timer", Source: lifecycle.SourceFunction, Route: "timer",
			Deadline: clock.Now().Add(time.Hour),
		})
	}()
	select {
	case <-workEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "deadline-timer work did not enter")
	}
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Kernel did not arm the deadline timer")
	}
	for index := range 32 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := kernel.Cancel(ctx, fmt.Sprintf("absent-%d", index))
		cancel()
		require.NoError(t, err)
	}

	arms := clock.deadlineArmCount()
	require.EqualValues(t, 1, arms)

	close(releaseWork)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "deadline-timer operation did not finish")
	}
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	closeUIDLedger(t, uids)
}

func TestKernelStartsQueuedCooperativeFunctionAfterItsDeadline(t *testing.T) {
	clock := newKernelFinalizerClock()
	release := make(chan struct{})
	blockerEntered := make(chan struct{}, 1)
	type deadlineObservation struct {
		deadline time.Time
		ok       bool
		err      error
		cause    error
	}
	deadlineEntered := make(chan deadlineObservation, 1)
	var deadlineCalls atomic.Int32
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "deadline" {
			return WorkPlan{
				CooperativeDeadline: true,
				Work: frameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					deadlineCalls.Add(1)
					deadline, ok := ctx.Deadline()
					deadlineEntered <- deadlineObservation{deadline: deadline, ok: ok, err: ctx.Err(), cause: context.Cause(ctx)}
					return lifecycle.NewControlResult(lifecycle.ControlDeadline)
				}),
			}, nil
		}
		return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			blockerEntered <- struct{}{}
			<-release
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "queued-deadline"
	})

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	future := clock.Now().Add(time.Minute)

	require.NoError(t, kernel.Submit(context.Background(), Request{
		UID: "blocker", Source: lifecycle.SourceFunction,
		Route: "blocker", Deadline: future,
	}),
	)

	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane blocker did not start")
	}
	due := clock.Now()

	require.NoError(t, kernel.Submit(context.Background(), Request{
		UID: "queued-deadline", Source: lifecycle.SourceFunction,
		Route: "deadline", Deadline: due,
	}),
	)

	select {
	case observed := <-deadlineEntered:
		require.FailNowf(t, "test failed", "queued deadline handler bypassed its active lane: %+v", observed)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	var observed deadlineObservation
	seen := false
	select {
	case observed = <-deadlineEntered:
		seen = true
	case <-time.After(time.Second):
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	require.True(t, seen)

	calls := deadlineCalls.Load()
	require.False(t, calls != 1 || !observed.ok || !observed.deadline.Equal(due) ||
		!errors.Is(observed.err, context.DeadlineExceeded) || !errors.Is(observed.cause, context.DeadlineExceeded))

	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN queued-deadline 504 application/json ")))
	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0)

	closeUIDLedger(t, uids)
}

func TestKernelStartsDueCooperativeRunner(t *testing.T) {
	clock := newKernelFinalizerClock()
	release := make(chan struct{})
	blockerEntered := make(chan struct{}, 1)
	observed := make(chan error, 1)
	runner := kernelDeadlineRunner{observed: observed}
	planner := plannerFunc(func(
		_ context.Context,
		route string,
		_ []string,
	) (WorkPlan, error) {
		if route == "runner" {
			return WorkPlan{
				Work:                runner.RunTask,
				CooperativeDeadline: true,
			}, nil
		}
		return WorkPlan{
			Work: frameTaskWork(func(
				context.Context,
			) (lifecycle.SealedResult, error) {
				blockerEntered <- struct{}{}
				<-release
				return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
			}),
		}, nil
	})
	var output bytes.Buffer
	kernel, run, uids, tasks :=
		newKernelWithClockFinalizerAndTimeout(
			t,
			planner,
			&output,
			clock,
			newNoopRunFinalizer(),
			time.Second,
		)

	require.NoError(t, run.OpenAdmission())

	setTestFunctionResource(t, kernel, func(lookup FunctionLookup) string {
		return "due-runner"
	})
	startKernelLoop(t, kernel)

	require.NoError(t, kernel.Submit(
		context.Background(),
		Request{
			UID:      "runner-blocker",
			Source:   lifecycle.SourceFunction,
			Route:    "blocker",
			Deadline: clock.Now().Add(time.Minute),
		},
	),
	)

	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane runner blocker did not start")
	}
	result := make(chan error, 1)
	go func() {
		result <- kernel.submitAndWait(
			context.Background(),
			Request{
				UID:      "due-runner",
				Source:   lifecycle.SourceFunction,
				Route:    "runner",
				Deadline: clock.Now(),
			},
		)
	}()
	select {
	case cause := <-observed:
		require.FailNowf(t, "test failed", "due cooperative runner bypassed its active lane: %v", cause)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case cause := <-observed:
		require.ErrorIs(t, cause, context.DeadlineExceeded)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "due cooperative runner was terminalized without execution")
	}
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "due cooperative runner did not reach terminal disposition")
	}
	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN due-runner 504 application/json ")))
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)

	closeUIDLedger(t, uids)
}

type kernelDeadlineRunner struct {
	observed chan<- error
}

func (kdr kernelDeadlineRunner) RunTask(
	ctx context.Context,
) (lifecycle.TaskOutcome, error) {
	kdr.observed <- context.Cause(ctx)
	result, err := lifecycle.NewControlResult(lifecycle.ControlDeadline)
	if err != nil {
		return lifecycle.TaskOutcome{}, err
	}
	return lifecycle.NewFrameOutcome(result)
}

func TestKernelSchedulesExpiredCooperativeFunctionAfterItsLanePredecessor(t *testing.T) {
	clock := newKernelFinalizerClock()
	releasePredecessor := make(chan struct{})
	predecessorEntered := make(chan struct{}, 1)
	type deadlineObservation struct {
		deadline time.Time
		ok       bool
		err      error
		cause    error
	}
	deadlineEntered := make(chan deadlineObservation, 1)
	var deadlineCalls atomic.Int32
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "deadline" {
			return WorkPlan{
				CooperativeDeadline: true,
				Work: frameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					deadlineCalls.Add(1)
					deadline, ok := ctx.Deadline()
					deadlineEntered <- deadlineObservation{deadline: deadline, ok: ok, err: ctx.Err(), cause: context.Cause(ctx)}
					return lifecycle.NewControlResult(lifecycle.ControlDeadline)
				}),
			}, nil
		}
		return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			predecessorEntered <- struct{}{}
			<-releasePredecessor
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string { return "same-lane" })

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	predecessorResult := make(chan error, 1)
	go func() {
		predecessorResult <- kernel.submitAndWait(context.Background(), Request{
			UID: "same-lane-predecessor", Source: lifecycle.SourceFunction, Route: "predecessor",
		})
	}()
	select {
	case <-predecessorEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane predecessor did not start")
	}
	due := clock.Now().Add(time.Second)
	deadlineResult := make(chan error, 1)
	go func() {
		deadlineResult <- kernel.submitAndWait(context.Background(), Request{
			UID: "same-lane-deadline", Source: lifecycle.SourceFunction,
			Route: "deadline", Deadline: due,
		})
	}()
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane successor deadline was not armed")
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	barrierCtx, barrierCancel := context.WithTimeout(context.Background(), time.Second)
	defer barrierCancel()

	require.NoError(t, kernel.Cancel(barrierCtx, "same-lane-deadline"))

	select {
	case observed := <-deadlineEntered:
		require.FailNowf(t, "test failed", "same-lane deadline handler bypassed its predecessor: %+v", observed)
	default:
	}
	close(releasePredecessor)
	select {
	case err := <-predecessorResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane predecessor did not complete")
	}
	var observed deadlineObservation
	select {
	case observed = <-deadlineEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "expired same-lane successor was not scheduled")
	}
	select {
	case err := <-deadlineResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane deadline operation did not complete")
	}
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	calls := deadlineCalls.Load()
	require.False(t, calls != 1 || !observed.ok || !observed.deadline.Equal(due) ||
		!errors.Is(observed.err, context.DeadlineExceeded) || !errors.Is(observed.cause, context.DeadlineExceeded))

	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN same-lane-deadline 504 application/json ")))
	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0)

	closeUIDLedger(t, uids)
}

func TestKernelCancelBeforeQueuedCooperativeDeadlineDisposesOperation(t *testing.T) {
	clock := newKernelFinalizerClock()
	releaseHolder := make(chan struct{})
	holderEntered := make(chan struct{}, 1)
	var deadlineWorkStarted atomic.Bool
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "deadline" {
			return WorkPlan{
				CooperativeDeadline: true,
				Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
					deadlineWorkStarted.Store(true)
					return lifecycle.NewControlResult(lifecycle.ControlDeadline)
				}),
			}, nil
		}
		return WorkPlan{
			Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
				holderEntered <- struct{}{}
				<-releaseHolder
				return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
			}),
		}, nil
	})
	var output bytes.Buffer
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(
		t,
		planner,
		&output,
		clock,
		newNoopRunFinalizer(),
		time.Second,
	)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "cancel-before-deadline"
	})

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	t.Cleanup(func() {
		kernel.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = kernel.Wait(ctx)
	})
	holderResult := make(chan error, 1)
	require.NoError(t, kernel.submit(context.Background(), Request{
		UID: "cancel-deadline-holder", Source: lifecycle.SourceFunction,
		Route: "holder",
	}, holderResult))
	select {
	case <-holderEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane holder did not start")
	}

	due := clock.Now().Add(time.Second)
	deadlineResult := make(chan error, 1)
	require.NoError(t, kernel.submit(context.Background(), Request{
		UID: "cancel-before-deadline", Source: lifecycle.SourceFunction,
		Route: "deadline", Deadline: due,
	}, deadlineResult))
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "queued cooperative deadline was not armed")
	}

	clock.advance(time.Second + time.Nanosecond)
	require.NoError(t, kernel.Cancel(context.Background(), "cancel-before-deadline"))
	close(releaseHolder)
	select {
	case err := <-holderResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "holder operation did not finish")
	}
	select {
	case err := <-deadlineResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"cancelled cooperative-deadline operation retained ownership",
		)
	}

	require.False(t, deadlineWorkStarted.Load())

	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.Empty(t, kernel.operations)
	require.Empty(t, kernel.lanes)
	require.EqualValues(t, 0, tasks.Active())
	require.EqualValues(t, 0, tasks.Pending())

	closeUIDLedger(t, uids)
}

func TestKernelDisposesQueuedNonCooperativeWorkAfterItsDeadline(t *testing.T) {
	clock := newKernelFinalizerClock()
	releaseBlockers := make(chan struct{})
	blockerEntered := make(chan struct{}, 1)
	var workCalls atomic.Int32
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "noncooperative" {
			return WorkPlan{
				Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
					workCalls.Add(1)
					return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
				}),
			}, nil
		}
		return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			blockerEntered <- struct{}{}
			<-releaseBlockers
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "queued-noncooperative"
	})

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.Submit(context.Background(), Request{
		UID:    "noncooperative-blocker",
		Source: lifecycle.SourceFunction, Route: "blocker",
	}),
	)

	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane noncooperative blocker did not start")
	}
	terminal := make(chan error, 1)
	go func() {
		terminal <- kernel.submitAndWait(context.Background(), Request{
			UID: "queued-noncooperative-deadline", Source: lifecycle.SourceFunction,
			Route: "noncooperative", Deadline: clock.Now(),
		})
	}()
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "queued noncooperative deadline was not armed")
	}
	kernel.NotifyControlReady()
	select {
	case err := <-terminal:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "queued noncooperative deadline did not reach terminal disposal")
	}
	require.EqualValues(t, 0, workCalls.Load())
	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN queued-noncooperative-deadline 504 application/json ")))
	close(releaseBlockers)
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0)

	closeUIDLedger(t, uids)
}

func TestKernelOneRetainedTimeoutPlusThreeActiveTasksDoesNotDirty(t *testing.T) {
	clock := newKernelFinalizerClock()
	entered := make(chan string, lifecycle.RetainedTimeoutFailStopThreshold)
	releaseWork := make(chan struct{})
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			entered <- route
			<-releaseWork
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	writer := &firstHoldingFrameWriter{offered: make(chan []byte, 1), release: make(chan struct{})}
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, writer, clock, newNoopRunFinalizer(), time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	deadline := clock.Now().Add(time.Second)
	terminals := make([]chan error, lifecycle.RetainedTimeoutFailStopThreshold)
	for index := range lifecycle.RetainedTimeoutFailStopThreshold {
		terminals[index] = make(chan error, 1)
		request := Request{
			UID:    fmt.Sprintf("mixed-retained-%d", index),
			Source: lifecycle.SourceFunction, Route: fmt.Sprintf("work-%d", index),
		}
		if index == 0 {
			request.Deadline = deadline
		}

		require.NoError(t, kernel.submit(context.Background(), request, terminals[index]))
	}
	for range lifecycle.RetainedTimeoutFailStopThreshold {
		select {
		case <-entered:
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "mixed retained TaskChild did not start")
		}
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	select {
	case frame := <-writer.offered:
		require.True(t, bytes.Contains(frame, []byte("FUNCTION_RESULT_BEGIN mixed-retained-0 504 application/json ")))
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "mixed retained timeout frame was not offered")
	}
	close(writer.release)
	barrierCtx, barrierCancel := context.WithTimeout(context.Background(), time.Second)
	defer barrierCancel()

	require.NoError(t, kernel.Cancel(barrierCtx, "retained-count-barrier"))

	require.NoError(t, run.DirtyCause())

	close(releaseWork)
	for index, terminal := range terminals {
		select {
		case err := <-terminal:
			require.NoError(t, err)
		case <-time.After(time.Second):
			require.FailNowf(t, "test failed", "mixed retained terminal %d did not complete", index)
		}
	}
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0)

	closeUIDLedger(t, uids)
}

func TestKernelFourthBackgroundTransactionTimeoutDirtiesWithoutResponseCommit(t *testing.T) {
	clock := newKernelFinalizerClock()
	release := make(chan struct{})
	entered := make(chan string, lifecycle.RetainedTimeoutFailStopThreshold)
	planner := plannerFunc(func(_ context.Context, route string, args []string) (WorkPlan, error) {
		if route != "background-transaction" || len(args) != 1 {
			return WorkPlan{}, errors.New("unexpected background transaction request")
		}
		id := args[0]
		return WorkPlan{
			NoResponse: true, CooperativeDeadline: true,
			Transaction: &ResourceTransactionPlan{
				ID: id,
				Prepare: func(
					_ context.Context,
					_ lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					_ lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					entered <- id
					<-release
					return &simpleCompositeChildTransaction{scope: scope}, nil
				},
			},
		}, nil
	})
	var output bytes.Buffer
	kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	deadline := clock.Now().Add(time.Second)
	terminals := make([]chan error, lifecycle.RetainedTimeoutFailStopThreshold)
	for index := range lifecycle.RetainedTimeoutFailStopThreshold {
		id := fmt.Sprintf("job:background-%d", index)
		terminals[index] = make(chan error, 1)

		require.NoError(t, kernel.submit(context.Background(), Request{
			UID: fmt.Sprintf("background-timeout-%d", index), LaneKey: id, Source: lifecycle.SourceJobManager,
			Route: "background-transaction", Args: []string{id}, Deadline: deadline,
		}, terminals[index]),
		)
	}
	for range lifecycle.RetainedTimeoutFailStopThreshold {
		select {
		case <-entered:
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "background transaction did not occupy its TaskChild slot")
		}
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	require.Eventually(t, func() bool {
		cause := run.DirtyCause()
		return cause != nil && strings.Contains(
			cause.Error(),
			"fourth background timeout reached the retained-timeout fail-stop threshold",
		)
	}, time.Second, time.Millisecond)

	cause := run.DirtyCause()
	require.False(t, cause == nil || !strings.Contains(cause.Error(), "fourth background timeout reached the retained-timeout fail-stop threshold"))

	close(release)
	for index, terminal := range terminals {
		select {
		case err := <-terminal:
			require.NoError(t, err)
		case <-time.After(time.Second):
			require.FailNowf(t, "test failed", "background terminal %d did not complete", index)
		}
	}
	terminalErr := kernel.Wait(context.Background())
	require.False(t, terminalErr == nil || !strings.Contains(terminalErr.Error(), "fourth background timeout reached the retained-timeout fail-stop threshold"))
	require.EqualValues(t, 0, output.Len())

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 || tasks.LongLivedCensus() != (lifecycle.LongLivedCensus{}))

	closeUIDLedger(t, uids)
}

func TestKernelRunFinalizerReleasesOnlyTypedFinalizerOwnedPermit(t *testing.T) {
	var permit lifecycle.LongLivedPermit
	called := make(chan struct{}, 1)
	finalizer := runFinalizerFunc(func(context.Context, uint64) error {
		called <- struct{}{}
		if err := permit.ReleaseExternal(); err != nil {
			return err
		}
		return permit.Return()
	})
	kernel, run, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)
	plan := lifecycle.NewSecretStoreLongLivedPlan()
	var err error
	permit, err = tasks.IssueLongLivedPermit(
		lifecycle.ResourceIdentity{ID: "secret-store", Generation: 1},
		plan,
	)
	require.NoError(t, err)

	require.NoError(t, permit.ActivateExternal())

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	select {
	case <-called:
	default:
		require.FailNow(t, "test failed", "typed finalizer-owned permit prevented finalizer dispatch")
	}

	census := tasks.LongLivedCensus()
	require.EqualValues(t, lifecycle.LongLivedCensus{}, census)

	terminal := run.TerminalState()
	require.False(t, !terminal.Quiescent || terminal.Dirty != nil)
}

func TestKernelRunFinalizerFailureReleasesTaskWithoutQuiescence(t *testing.T) {
	want := errors.New("finalizer failed")
	finalizer := runFinalizerFunc(func(context.Context, uint64) error { return want })
	kernel, run, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()

	err := kernel.Wait(context.Background())
	require.ErrorIs(t, err, want)

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)
	terminal := run.TerminalState()
	require.False(t, !terminal.Reached || terminal.Quiescent || !errors.Is(terminal.Dirty, want))
}

func TestKernelRunFinalizerPanicReleasesTaskWithoutQuiescence(t *testing.T) {
	finalizer := runFinalizerFunc(func(context.Context, uint64) error { panic("finalizer panic") })
	kernel, run, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()

	err := kernel.Wait(context.Background())
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)
	terminal := run.TerminalState()
	require.False(t, !terminal.Reached || terminal.Quiescent || !errors.Is(terminal.Dirty, lifecycle.ErrTaskPanic))
}

func TestKernelRunFinalizerRejectsUnrelatedLongLivedPermit(t *testing.T) {
	var calls atomic.Int32
	finalizer := runFinalizerFunc(func(context.Context, uint64) error {
		calls.Add(1)
		return nil
	})
	kernel, run, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, 10*time.Millisecond)
	plan := lifecycle.NewJobLongLivedPlan()
	permit, err := tasks.IssueLongLivedPermit(
		lifecycle.ResourceIdentity{ID: "job", Generation: 1},
		plan,
	)
	require.NoError(t, err)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()

	waitErr := kernel.Wait(context.Background())
	require.False(t, waitErr == nil || !strings.Contains(waitErr.Error(), "shutdown deadline exceeded"))

	require.EqualValues(t, 0, calls.Load())

	require.NoError(t, permit.AbortUnused())
}

func TestKernelRejectsMissingRunFinalizer(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, time.Second)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	planner := stoppedKernelPlanner{}

	_, newCommandKernelErr := NewCommandKernel(
		run, uids, tasks, frames, lifecycle.RealClock{},
		newNoopRunShutdownBarrier(), nil,
		newTestFunctionCatalog(planner),
	)
	require.Error(t, newCommandKernelErr)

}

func TestKernelCannotReportQuiescentWithRetainedLongLivedPermit(t *testing.T) {
	kernel, run, _, tasks := newKernelWithPlannerAndTimeout(t, stoppedKernelPlanner{}, time.Millisecond)
	permitPlan := lifecycle.NewJobLongLivedPlan()
	permit, err := tasks.IssueLongLivedPermit(
		lifecycle.ResourceIdentity{ID: "retained", Generation: 1}, permitPlan,
	)
	require.NoError(t, err)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	waitErr := kernel.Wait(ctx)
	require.False(t, waitErr == nil || !strings.Contains(waitErr.Error(), "shutdown deadline exceeded") || !strings.Contains(waitErr.Error(), "nonzero process census"))

	require.EqualValues(t, 1, tasks.LongLivedCensus().Active)

	require.NoError(t, permit.AbortUnused())

}

func TestKernelStopDrainsCooperativeTask(t *testing.T) {
	started := make(chan struct{})
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			CooperativeCancel: true,
			Work: frameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
				close(started)
				<-ctx.Done()
				return lifecycle.NewSealedResult(499, "application/json", []byte(`{"status":499}`))
			}),
		}, nil
	})
	kernel, run, uids, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string { return "lane" })

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.Submit(context.Background(), Request{
		UID: "cooperative-stop", Source: lifecycle.SourceFunction,
		Route: "route", Deadline: time.Now().Add(time.Minute),
	}),
	)

	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "cooperative task did not start")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	require.False(t, tasks.Active() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0)

	closeUIDLedger(t, uids)
}

func TestKernelShutdownCancelsOperationsInBoundedTurns(t *testing.T) {
	const (
		population = 257
		quantum    = 4
	)
	kernel, run := newKernel(t)

	require.NoError(t, run.OpenAdmission())

	plan := WorkPlan{
		Work:       frameTaskWork(plannerPlanWork),
		NoResponse: true,
	}
	for index := range population {
		request := Request{
			UID:     fmt.Sprintf("shutdown-operation-%d", index),
			Source:  lifecycle.SourceJobManager,
			LaneKey: "shared",
		}

		require.NoError(t, kernel.admit(request, plan))
	}

	require.NoError(t, kernel.beginShutdown(time.Now().Add(time.Second)))

	for {
		before := len(kernel.operations)
		more, err := kernel.serviceShutdownCancellation(quantum)
		require.NoError(t, err)
		visited := before - len(kernel.operations)
		require.False(t, visited < 0 || visited > quantum)
		if !more {
			break
		}
	}
	require.False(t, len(kernel.operations) != 0 || kernel.operationHead != nil || kernel.operationTail != nil)

	require.NoError(t, kernel.advanceShutdownAuthority())
	completeNoopShutdownBarrier(t, kernel)

	_, err := kernel.serviceShutdownStops(quantum)
	require.NoError(t, err)

	require.False(t, len(kernel.lanes) != 0 || kernel.laneHead != nil || kernel.laneTail != nil)
}

func TestKernelShutdownVisitsLiveLanesOnceInBoundedTurns(t *testing.T) {
	populations := map[string]int{
		"one":                     1,
		"thirty-two":              32,
		"two-hundred-fifty-seven": 257,
	}
	const quantum = 4
	for name, population := range populations {
		t.Run(name, func(t *testing.T) {
			kernel, run := newKernel(t)

			require.NoError(t, run.OpenAdmission())

			for index := range population {
				key := fmt.Sprintf("shutdown-lane-%d", index)

				_, err := kernel.allocateLane(
					commandLaneKey{
						source: lifecycle.SourceJobManager,
						key:    key,
					},
					Request{
						Source:  lifecycle.SourceJobManager,
						LaneKey: key,
					},
				)
				require.NoError(t, err)
			}

			require.NoError(t, kernel.beginShutdown(time.Now().Add(time.Second)))
			require.NoError(t, kernel.advanceShutdownAuthority())
			completeNoopShutdownBarrier(t, kernel)

			for {
				before := len(kernel.lanes)
				more, err := kernel.serviceShutdownStops(quantum)
				require.NoError(t, err)
				visited := before - len(kernel.lanes)
				require.False(t, visited < 0 || visited > quantum)
				if !more {
					break
				}
			}
			require.False(t, len(kernel.lanes) != 0 || kernel.laneHead != nil || kernel.laneTail != nil)
		})
	}
}

func completeNoopShutdownBarrier(t *testing.T, kernel *testCommandKernel) {
	t.Helper()
	require.NoError(t, kernel.advanceShutdownBarrier())
	kernel.serviceTaskStarts(1)
	select {
	case completion := <-kernel.tasks.CompletionCh():
		kernel.completeTask(completion)
	case <-time.After(time.Second):
		require.FailNow(t, "shutdown barrier did not complete")
	}
	select {
	case acknowledgement := <-kernel.tasks.AcknowledgementCh():
		kernel.acknowledgeTask(acknowledgement)
	case <-time.After(time.Second):
		require.FailNow(t, "shutdown barrier termination was not acknowledged")
	}
	require.True(t, kernel.shutdownBarrierDone)
}

func TestKernelCancelsQueuedOperationWithoutStartingWork(t *testing.T) {
	started := make(chan struct{})
	var queuedWorkStarted atomic.Bool
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		switch route {
		case "blocking":
			return WorkPlan{
				CooperativeCancel: true,
				Work: frameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					close(started)
					<-ctx.Done()
					return lifecycle.NewControlResult(lifecycle.ControlCancelled)
				}),
			}, nil
		case "queued":
			return WorkPlan{
				Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
					queuedWorkStarted.Store(true)
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				}),
			}, nil
		default:
			return WorkPlan{}, errors.New("unexpected route")
		}
	})
	kernel, run, uids, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string { return "lane" })
	catalog := testFunctionCatalogFor(t, kernel)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	deadline := time.Now().Add(time.Minute)

	require.NoError(t, kernel.Submit(context.Background(), Request{UID: "blocking", Source: lifecycle.SourceFunction, Route: "blocking", Deadline: deadline}))

	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "blocking operation did not start")
	}
	queuedResult := make(chan error, 1)

	require.NoError(t, kernel.submit(context.Background(), Request{
		UID: "queued", Source: lifecycle.SourceFunction, Route: "queued", Deadline: deadline,
	}, queuedResult),
	)

	require.NoError(t, kernel.Cancel(context.Background(), "queued"))

	select {
	case err := <-queuedResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "queued cancellation did not reach terminal disposal")
	}
	require.False(t, queuedWorkStarted.Load())
	require.False(t, catalog.next != 2 || catalog.release != 1 || len(catalog.leases) != 1)

	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)
	require.False(t, catalog.next != 2 || catalog.release != 2 || len(catalog.leases) != 0)

	closeUIDLedger(t, uids)
}

func TestKernelWritesLargeValidResult(t *testing.T) {
	sealed := largeRawSealedResult(t)
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) { return sealed, nil })}, nil
	})
	writer := &holdingFrameWriter{offered: make(chan []byte, 1), release: make(chan struct{})}
	kernel, run, uids, _ := newKernelWithPlannerAndWriter(t, planner, writer)
	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)
	request := Request{UID: "self-fit", Source: lifecycle.SourceFunction, Route: "route"}

	require.NoError(t, kernel.Submit(context.Background(), request))

	select {
	case <-writer.offered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "result did not reach held Write")
	}

	close(writer.release)
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	closeUIDLedger(t, uids)
}

func plannerPlanWork(context.Context) (lifecycle.SealedResult, error) {
	return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
}

func largeRawSealedResult(t *testing.T) lifecycle.SealedResult {
	t.Helper()
	payload := []byte(`{"pad":"` + strings.Repeat("A", 1024*1024) + `"}`)
	result, err := lifecycle.NewSealedResult(200, "application/json", payload)
	require.NoError(t, err)
	return result
}

type holdingFrameWriter struct {
	offered chan []byte
	release chan struct{}
}

func (hfw *holdingFrameWriter) Write(payload []byte) (int, error) {
	hfw.offered <- bytes.Clone(payload)
	<-hfw.release
	return len(payload), nil
}

type firstHoldingFrameWriter struct {
	once    sync.Once
	offered chan []byte
	release chan struct{}
}

func (fhfw *firstHoldingFrameWriter) Write(payload []byte) (int, error) {
	fhfw.once.Do(func() {
		fhfw.offered <- bytes.Clone(payload)
		<-fhfw.release
	})
	return len(payload), nil
}

func TestKernelExternalSubmissionServiceRotatesSources(t *testing.T) {
	kernel, run := newKernel(t)

	require.NoError(t, run.OpenAdmission())

	requests := []Request{
		{UID: "j1", LaneKey: "j1", Source: lifecycle.SourceJobManager, Route: "route"},
		{UID: "j2", LaneKey: "j2", Source: lifecycle.SourceJobManager, Route: "route"},
		{UID: "f1", Source: lifecycle.SourceFunction, Route: "route"},
		{UID: "f2", Source: lifecycle.SourceFunction, Route: "route"},
	}
	results := make([]chan error, len(requests))
	for index, request := range requests {
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		require.NoError(t, err)
		results[index] = make(chan error, 1)
		kernel.submissions[sourceIndex(request.Source)] <- submission{request: request, plan: plan, result: results[index]}
	}
	kernel.serviceSubmissions(4)
	for _, result := range results {
		require.NoError(t, <-result)
	}
	want := map[string]lifecycle.OperationID{"j1": 1, "f1": 2, "j2": 3, "f2": 4}
	for uid, id := range want {
		operation := kernel.operations[uid]
		require.False(t, operation == nil || operation.ID != id)
	}
}

func TestKernelShutdownDrainsMoreThanTwoSubmissionQuantaWithoutAnotherWake(t *testing.T) {
	kernel, _ := newKernel(t)
	const count = 9
	results := make([]chan error, count)
	for index := range results {
		results[index] = make(chan error, 1)
		request := Request{
			UID:    fmt.Sprintf("queued-%d", index),
			Source: lifecycle.SourceFunction,
			Route:  "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		require.NoError(t, err)
		kernel.submissions[sourceIndex(lifecycle.SourceFunction)] <- submission{
			request: request,
			plan:    plan,
			result:  results[index],
		}
	}

	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	for index, result := range results {
		select {
		case err := <-result:
			stopping, ok := errors.AsType[*lifecycle.StoppingRejection](err)
			require.True(t, ok)
			require.EqualValues(t, 1, stopping.Generation)
		default:
			require.FailNowf(t, "test failed", "submission %d was not drained", index)
		}
	}
}

func TestKernelClosedAdmissionDoesNotRearmFrameBlockedControl(t *testing.T) {
	kernel, _ := newKernel(t)
	source := sourceIndex(lifecycle.SourceFunction)
	kernel.hasBlockedSubmission[source] = true
	kernel.blockedSubmissions[source] = submission{controlStatus: lifecycle.ControlBadRequest}
	require.False(t, kernel.hasRunnableSubmissions())
}

func TestKernelTaskSchedulingCountsClaimConflictsAgainstQuantum(t *testing.T) {
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			Claims: []string{"shared"},
			Work:   frameTaskWork(plannerPlanWork),
		}, nil
	})
	kernel, run, _, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(lookup FunctionLookup) string { return lookup.UID })

	require.NoError(t, run.OpenAdmission())

	for index := range 9 {
		request := Request{
			UID:    fmt.Sprintf("claim-%d", index),
			Source: lifecycle.SourceFunction,
			Route:  "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		require.NoError(t, err)

		require.NoError(t, kernel.admit(request, plan))
	}
	got := kernel.ready[0].len + kernel.ready[1].len
	require.EqualValues(t, 9, got)

	require.False(t, !kernel.scheduleTasks(4) ||
		tasks.Pending() != 1 || kernel.claims.waitingCount() != 3 || kernel.ready[0].len+kernel.ready[1].len != 5)

	require.False(t, !kernel.scheduleTasks(4) ||
		tasks.Pending() != 1 || kernel.claims.waitingCount() != 7 || kernel.ready[0].len+kernel.ready[1].len != 1)

	require.False(t, kernel.scheduleTasks(4) ||
		tasks.Pending() != 1 || kernel.claims.waitingCount() != 8 || kernel.ready[0].len+kernel.ready[1].len != 0)
}

func TestKernelResourceScopedFunctionHasIndependentTaskSchedulingClass(t *testing.T) {
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: frameTaskWork(plannerPlanWork)}, nil
	})
	kernel, run, _, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(lookup FunctionLookup) string {
		if lookup.Route == "dyncfg" {
			return "dyncfg-resource"
		}
		return ""
	})

	require.NoError(t, run.OpenAdmission())

	requests := []Request{
		{UID: "generic-first", Source: lifecycle.SourceFunction, Route: "generic"},
		{UID: "dyncfg-second", Source: lifecycle.SourceFunction, Route: "dyncfg"},
	}
	for _, request := range requests {
		require.NoError(t, kernel.admit(request, WorkPlan{}))
	}
	more := kernel.scheduleTasks(len(requests))
	require.False(t, more)

	require.EqualValues(t, len(requests), tasks.Pending())
	var started [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, _, err := tasks.Dispatch(context.Background(), 1, &started)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	operation := kernel.tasksByRequest[started[0].Request]
	require.NotNil(t, operation)
	require.EqualValues(t, lifecycle.SourceFunction, operation.Source)
	require.EqualValues(t, "dyncfg-second", operation.request.UID)
}

func TestKernelSubmissionBacklogCannotStarveStop(t *testing.T) {
	kernel, run, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})

	require.NoError(t, run.OpenAdmission())

	for index := range externalSourceQueueDepth {
		request := Request{
			UID:    fmt.Sprintf("backlog-%d", index),
			Source: lifecycle.SourceFunction, Route: "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		require.NoError(t, err)
		kernel.submissions[sourceIndex(request.Source)] <- submission{
			request: request, plan: plan, result: make(chan error, 1),
		}
	}
	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	closeUIDLedger(t, uids)
}

func TestKernelSubmissionBacklogCannotStarveDueDeadline(t *testing.T) {
	var output bytes.Buffer
	kernel, run, uids, _ := newKernelWithPlannerAndWriter(t, stoppedKernelPlanner{}, &output)

	require.NoError(t, run.OpenAdmission())

	request := Request{
		UID: "deadline-probe", Source: lifecycle.SourceFunction, Route: "route",
		Deadline: time.Now().Add(-time.Second),
	}
	plan, err := kernel.prepareSubmissionPlanForTest(request)
	require.NoError(t, err)

	require.NoError(t, kernel.admit(request, plan))

	for index := range externalSourceQueueDepth {
		request := Request{
			UID:    fmt.Sprintf("backlog-%d", index),
			Source: lifecycle.SourceFunction, Route: "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		require.NoError(t, err)
		kernel.submissions[sourceIndex(request.Source)] <- submission{
			request: request, plan: plan, result: make(chan error, 1),
		}
	}
	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN deadline-probe 504 application/json ")))

	closeUIDLedger(t, uids)
}

func TestKernelPreAdmissionRejectionCommitsWithoutUIDOwnership(t *testing.T) {
	var output bytes.Buffer
	kernel, run, uids, _ := newKernelWithPlannerAndWriter(t, stoppedKernelPlanner{}, &output)

	require.NoError(t, run.OpenAdmission())

	startKernelLoop(t, kernel)

	require.NoError(t, kernel.Reject(context.Background(), "malformed-safe-uid", lifecycle.ControlBadRequest))

	active, _, _ := uids.Census()
	require.Zero(t, active)

	require.True(t, bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN malformed-safe-uid 400 application/json ")))
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, kernel.Wait(ctx))

	closeUIDLedger(t, uids)
}

func TestKernelDeadlineServiceHasFixedQuantum(t *testing.T) {
	kernel, _ := newKernel(t)
	now := time.Now()
	for index := range 9 {
		id := lifecycle.OperationID(index + 1)
		generation, err := lifecycle.NewOperation(id, fmt.Sprintf("u%d", index), lifecycle.SourceFunction, fmt.Sprintf("lane%d", index), true)
		require.NoError(t, err)
		for _, state := range []lifecycle.OperationState{
			lifecycle.OperationQueued, lifecycle.OperationAcquiringClaims, lifecycle.OperationReady, lifecycle.OperationRunning,
		} {
			require.NoError(t, generation.Advance(state))
		}

		require.NoError(t, generation.StartChild(lifecycle.TaskRef{Slot: uint32(index), Generation: uint64(index + 1)}))

		operation := &commandOperation{OperationGeneration: generation, deadline: deadlineEntry{when: now.Add(-time.Second), index: -1}}
		operation.deadline.operation = operation
		heap.Push(&kernel.deadlines, &operation.deadline)
	}

	require.False(t, !kernel.serviceDeadlines(now, 4) || len(kernel.controls) != 4 || kernel.deadlines.Len() != 5)

	require.False(t, !kernel.serviceDeadlines(now, 4) || len(kernel.controls) != 8 || kernel.deadlines.Len() != 1)

	require.False(t, kernel.serviceDeadlines(now, 4) || len(kernel.controls) != 9 || kernel.deadlines.Len() != 0)
}

func newStoppedKernel(t *testing.T) *testCommandKernel {
	t.Helper()
	kernel, _ := newKernel(t)
	startKernelLoop(t, kernel)
	kernel.Stop()

	require.NoError(t, kernel.Wait(context.Background()))

	return kernel
}

func newKernel(t *testing.T) (*testCommandKernel, *lifecycle.RunSupervisor) {
	t.Helper()
	kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	return kernel, run
}

func newKernelWithPlanner(t *testing.T, planner testPlanner) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerAndWriter(t, planner, io.Discard)
}

func newKernelWithPlannerAndTimeout(t *testing.T, planner testPlanner, timeout time.Duration) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerWriterAndTimeout(t, planner, io.Discard, timeout)
}

func newKernelWithPlannerAndWriter(t *testing.T, planner testPlanner, writer io.Writer) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerWriterAndTimeout(t, planner, writer, lifecycle.DefaultShutdownTimeout)
}

func newKernelWithPlannerWriterAndTimeout(t *testing.T, planner testPlanner, writer io.Writer, timeout time.Duration) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerWriterFinalizerAndTimeout(t, planner, writer, newNoopRunFinalizer(), timeout)
}

func newKernelWithPlannerWriterFinalizerAndTimeout(t *testing.T, planner testPlanner, writer io.Writer, finalizer RunFinalizer, timeout time.Duration) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithClockFinalizerAndTimeout(t, planner, writer, lifecycle.RealClock{}, finalizer, timeout)
}

func newKernelWithClockFinalizerAndTimeout(t *testing.T, planner testPlanner, writer io.Writer, clock lifecycle.Clock, finalizer RunFinalizer, timeout time.Duration) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithClockFinalizerCatalogAndTimeout(
		t, planner, newTestFunctionCatalog(planner), writer, clock, finalizer, timeout,
	)
}

func newKernelWithClockFinalizerCatalogAndTimeout(t *testing.T, planner testPlanner, functionCatalog FunctionCatalogPort, writer io.Writer, clock lifecycle.Clock, finalizer RunFinalizer, timeout time.Duration) (*testCommandKernel, *lifecycle.RunSupervisor, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	t.Helper()
	run, err := lifecycle.NewRunSupervisor(1, clock, timeout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	uids := lifecycle.NewUIDLedger()
	frames, err := lifecycle.NewFrameOwner(writer)
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	kernel, err := NewCommandKernel(
		run, uids, tasks, frames, clock,
		newNoopRunShutdownBarrier(), finalizer,
		functionCatalog,
	)
	require.NoError(t, err)
	return &testCommandKernel{CommandKernel: kernel, planner: planner}, run, uids, tasks
}

type kernelFinalizerClock struct {
	mu            sync.Mutex
	now           time.Time
	shutdown      chan time.Time
	deadlineArms  int
	deadlineArmed chan struct{}
}

func newKernelFinalizerClock() *kernelFinalizerClock {
	return &kernelFinalizerClock{now: time.Unix(100, 0), deadlineArmed: make(chan struct{}, 1)}
}

func (kfc *kernelFinalizerClock) Now() time.Time {
	kfc.mu.Lock()
	defer kfc.mu.Unlock()
	return kfc.now
}

func (kfc *kernelFinalizerClock) Arm(kind string, _ time.Duration) (<-chan time.Time, func()) {
	ready := make(chan time.Time, 1)
	kfc.mu.Lock()
	if kind == lifecycle.TimerKindShutdown {
		kfc.shutdown = ready
	} else if kind == lifecycle.TimerKindDeadline {
		kfc.deadlineArms++
		select {
		case kfc.deadlineArmed <- struct{}{}:
		default:
		}
	}
	kfc.mu.Unlock()
	return ready, func() {}
}

func (kfc *kernelFinalizerClock) deadlineArmCount() int {
	kfc.mu.Lock()
	defer kfc.mu.Unlock()
	return kfc.deadlineArms
}

func (kfc *kernelFinalizerClock) expireShutdown(t *testing.T) {
	t.Helper()
	kfc.mu.Lock()
	ready := kfc.shutdown
	kfc.now = kfc.now.Add(time.Second)
	now := kfc.now
	kfc.mu.Unlock()
	require.NotNil(t, ready)
	ready <- now
}

func (kfc *kernelFinalizerClock) advanceShutdownWithoutSignal(t *testing.T) {
	t.Helper()
	kfc.mu.Lock()
	defer kfc.mu.Unlock()
	require.NotNil(t, kfc.shutdown)
	kfc.now = kfc.now.Add(time.Second)
}

func (kfc *kernelFinalizerClock) advance(delay time.Duration) {
	kfc.mu.Lock()
	kfc.now = kfc.now.Add(delay)
	kfc.mu.Unlock()
}

func closeUIDLedger(t *testing.T, ledger *lifecycle.UIDLedger) {
	t.Helper()
	for {
		more, err := ledger.CloseBatch(lifecycle.UIDReturnBatch)
		require.NoError(t, err)
		if !more {
			return
		}
	}
}

type plannerFunc func(context.Context, string, []string) (WorkPlan, error)

func (fn plannerFunc) Plan(request Request) (WorkPlan, error) {
	return fn(context.Background(), request.Route, request.Args)
}

type testPlanner interface {
	Plan(Request) (WorkPlan, error)
}

type testCommandKernel struct {
	*CommandKernel
	planner testPlanner
}

func (tck *testCommandKernel) Submit(ctx context.Context, request Request) error {
	if request.Source != lifecycle.SourceJobManager {
		return tck.CommandKernel.Submit(ctx, request)
	}
	plan, err := tck.planner.Plan(request)
	if err != nil {
		return err
	}
	return tck.CommandKernel.SubmitPrepared(ctx, request, plan)
}

func (tck *testCommandKernel) submitAndWait(ctx context.Context, request Request) error {
	terminal := make(chan error, 1)
	if err := tck.submit(ctx, request, terminal); err != nil {
		return err
	}
	select {
	case err := <-terminal:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-tck.Done():
		return tck.Wait(context.Background())
	}
}

func (tck *testCommandKernel) submit(
	ctx context.Context,
	request Request,
	terminal chan error,
) error {
	if request.Source != lifecycle.SourceJobManager {
		return tck.CommandKernel.submit(ctx, request, terminal)
	}
	plan, err := tck.planner.Plan(request)
	if err != nil {
		return err
	}
	return tck.CommandKernel.submitPrepared(ctx, request, plan, terminal)
}

func (tck *testCommandKernel) prepareSubmissionPlanForTest(request Request) (WorkPlan, error) {
	if request.Source == lifecycle.SourceFunction {
		return WorkPlan{}, nil
	}
	plan, err := tck.planner.Plan(request)
	if err != nil {
		return WorkPlan{}, err
	}
	return prepareOwnedJobPlan(request, plan)
}

type testFunctionCatalog struct {
	planner  testPlanner
	resource func(FunctionLookup) string
	next     uint64
	release  uint64
	peak     int
	leases   map[FunctionInvocationRef]struct{}
}

type functionCatalogPortStub struct {
	resolve  func(FunctionLookup) (FunctionCatalogDecision, error)
	release  func(FunctionInvocationRef) (FunctionCleanupPlan, error)
	complete func(FunctionCleanupRef) error
}

func (fcps functionCatalogPortStub) ResolveAndAcquire(lookup FunctionLookup) (FunctionCatalogDecision, error) {
	return fcps.resolve(lookup)
}

func (fcps functionCatalogPortStub) ReleaseInvocation(ref FunctionInvocationRef) (FunctionCleanupPlan, error) {
	return fcps.release(ref)
}

func (fcps functionCatalogPortStub) CompleteCleanup(ref FunctionCleanupRef) error {
	if fcps.complete == nil {
		return nil
	}
	return fcps.complete(ref)
}

func (functionCatalogPortStub) BeginMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: mutations unsupported")
}

func (functionCatalogPortStub) AdvanceMutationQuiesce(int) (FunctionCatalogMutationProgress, error) {
	return FunctionCatalogMutationProgress{}, errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) ResumeMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) AdvanceMutation(int) (FunctionCatalogMutationProgress, []FunctionCleanupPlan, error) {
	return FunctionCatalogMutationProgress{}, nil, errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) AbortMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) BeginClose() error { return nil }

func (functionCatalogPortStub) CloseStep(int) ([]FunctionCleanupPlan, bool, error) {
	return nil, false, nil
}

func (functionCatalogPortStub) LifecycleDrained() bool { return true }

func newTestFunctionCatalog(planner testPlanner) *testFunctionCatalog {
	return &testFunctionCatalog{
		planner: planner,
		leases:  make(map[FunctionInvocationRef]struct{}),
	}
}

func (tfc *testFunctionCatalog) ResolveAndAcquire(lookup FunctionLookup) (FunctionCatalogDecision, error) {
	plan, err := tfc.planner.Plan(Request{
		UID: lookup.UID, Source: lifecycle.SourceFunction, Route: lookup.Route,
		Args: lookup.Args, Payload: lookup.Payload, ContentType: lookup.ContentType,
		Permissions: lookup.Permissions, CallerSource: lookup.CallerSource,
		Timeout: lookup.Timeout, HasPayload: lookup.HasPayload,
	})
	if err != nil {
		return FunctionCatalogDecision{}, err
	}
	tfc.next++
	ref := FunctionInvocationRef{Slot: 1, Generation: tfc.next}
	tfc.leases[ref] = struct{}{}
	if len(tfc.leases) > tfc.peak {
		tfc.peak = len(tfc.leases)
	}
	resourceID := ""
	if tfc.resource != nil {
		resourceID = tfc.resource(lookup)
	}
	return FunctionCatalogDecision{
		ResourceID: resourceID,
		Plan:       plan,
		Lease:      ref,
	}, nil
}

func (tfc *testFunctionCatalog) ReleaseInvocation(ref FunctionInvocationRef) (FunctionCleanupPlan, error) {
	if _, ok := tfc.leases[ref]; !ok {
		return FunctionCleanupPlan{}, errors.New("test Function catalog: unknown invocation lease")
	}
	delete(tfc.leases, ref)
	tfc.release++
	return FunctionCleanupPlan{}, nil
}

func (*testFunctionCatalog) CompleteCleanup(FunctionCleanupRef) error { return nil }

func (*testFunctionCatalog) BeginMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: mutations unsupported")
}

func (*testFunctionCatalog) AdvanceMutationQuiesce(int) (FunctionCatalogMutationProgress, error) {
	return FunctionCatalogMutationProgress{}, errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) ResumeMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) AdvanceMutation(int) (FunctionCatalogMutationProgress, []FunctionCleanupPlan, error) {
	return FunctionCatalogMutationProgress{}, nil, errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) AbortMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) BeginClose() error { return nil }

func (*testFunctionCatalog) CloseStep(int) ([]FunctionCleanupPlan, bool, error) {
	return nil, false, nil
}

func (*testFunctionCatalog) LifecycleDrained() bool { return true }

func setTestFunctionResource(t *testing.T, kernel *testCommandKernel, resource func(FunctionLookup) string) {
	t.Helper()
	testFunctionCatalogFor(t, kernel).resource = resource
}

func testFunctionCatalogFor(t *testing.T, kernel *testCommandKernel) *testFunctionCatalog {
	t.Helper()
	catalog, ok := kernel.functionCatalog.(*testFunctionCatalog)
	require.True(t, ok)
	return catalog
}

func kernelResourcePlanner(t *testing.T, resource *kernelTestReadyResource, workEntered chan<- struct{}, workRelease <-chan struct{}) testPlanner {
	t.Helper()
	permitPlan := lifecycle.NewJobLongLivedPlan()
	return kernelTestResourcePlanner{
		permitPlan:  permitPlan,
		resource:    resource,
		workEntered: workEntered,
		workRelease: workRelease,
	}
}

type kernelTestResourcePlanner struct {
	permitPlan  lifecycle.LongLivedPlan
	resource    *kernelTestReadyResource
	workEntered chan<- struct{}
	workRelease <-chan struct{}
}

type kernelTestResourceSetPlanner struct {
	permitPlan lifecycle.LongLivedPlan
	resources  map[string]*kernelTestReadyResource
}

func (ktrsp kernelTestResourceSetPlanner) Plan(request Request) (WorkPlan, error) {
	if request.Route != "install" {
		return WorkPlan{}, errors.New("unexpected kernel resource-set route")
	}
	resource := ktrsp.resources[request.LaneKey]
	if resource == nil {
		return WorkPlan{}, errors.New("unexpected kernel resource-set identity")
	}
	return kernelTestInstallPlan(request.LaneKey, ktrsp.permitPlan, resource), nil
}

type kernelTestTransactionPlanner struct {
	permitPlan          lifecycle.LongLivedPlan
	current             *kernelTestReadyResource
	successor           *kernelTestReadyResource
	prepareErr          error
	waitForCancellation bool
	events              *[]string
}

func (kttp kernelTestTransactionPlanner) Plan(
	request Request,
) (WorkPlan, error) {
	switch request.Route {
	case "install":
		return kernelTestInstallPlan(
			request.LaneKey,
			kttp.permitPlan,
			kttp.current,
		), nil
	case "replace":
		return WorkPlan{
			Transaction: &ResourceTransactionPlan{
				ID:                request.LaneKey,
				AllocateSuccessor: true,
				Permit:            kttp.permitPlan,
				Prepare: func(
					ctx context.Context,
					current lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					*kttp.events = append(*kttp.events, "prepare")
					if current != kttp.current ||
						scope.Current != kttp.current.identity ||
						!scope.Successor.Valid() {
						return nil, errors.New("kernel test transaction scope differs")
					}
					if kttp.waitForCancellation {
						<-ctx.Done()
					}
					if kttp.prepareErr != nil {
						return nil, kttp.prepareErr
					}
					return &kernelTestPreparedResourceTransaction{
						scope:     scope,
						current:   kttp.current,
						successor: kttp.successor,
						permit:    permit,
						events:    kttp.events,
					}, nil
				},
			},
		}, nil
	default:
		return WorkPlan{}, errors.New("unexpected kernel transaction route")
	}
}

type kernelTestPreparedResourceTransaction struct {
	scope     lifecycle.ResourceTransactionScope
	current   *kernelTestReadyResource
	successor *kernelTestReadyResource
	permit    lifecycle.LongLivedPermit
	events    *[]string
}

func (ktprt *kernelTestPreparedResourceTransaction) Scope() lifecycle.ResourceTransactionScope {
	return ktprt.scope
}

func (ktprt *kernelTestPreparedResourceTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	*ktprt.events = append(*ktprt.events, "apply")
	if err := ktprt.current.Stop(ctx); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if err := ktprt.current.Finalize(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if err := ktprt.permit.ActivateExternal(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	ktprt.successor.identity = ktprt.scope.Successor
	ktprt.successor.permit = ktprt.permit
	if err := ktprt.successor.Publish(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"accepted":true}`))
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		ktprt.scope,
		lifecycle.ResourceTransactionReplaced,
		ktprt.successor,
		result,
		func() error {
			*ktprt.events = append(*ktprt.events, "cleanup")
			return nil
		},
	)
}

func (ktprt *kernelTestPreparedResourceTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	*ktprt.events = append(*ktprt.events, "dispose")
	return ktprt.current, ktprt.permit.AbortUnused()
}

func (ktrp kernelTestResourcePlanner) Plan(request Request) (WorkPlan, error) {
	switch request.Route {
	case "install":
		return kernelTestInstallPlan(
			request.LaneKey,
			ktrp.permitPlan,
			ktrp.resource,
		), nil
	case "use":
		return WorkPlan{
			Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
				if ktrp.workEntered != nil {
					close(ktrp.workEntered)
				}
				if ktrp.workRelease != nil {
					<-ktrp.workRelease
				}
				return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
			}),
		}, nil
	case "stop":
		return kernelTestRemovePlan(request.LaneKey), nil
	default:
		return WorkPlan{}, errors.New("unexpected kernel resource route")
	}
}

func kernelTestInstallPlan(
	id string,
	permit lifecycle.LongLivedPlan,
	resource *kernelTestReadyResource,
) WorkPlan {
	return WorkPlan{
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: id, AllocateSuccessor: true, Permit: permit,
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil || scope.Current.Valid() ||
					scope.ID != id || !scope.Successor.Valid() {
					return nil, errors.New("kernel test install scope differs")
				}
				return &kernelTestInstallTransaction{
					scope: scope, permit: permit, resource: resource,
				}, nil
			},
		},
	}
}

func kernelTestRemovePlan(id string) WorkPlan {
	return WorkPlan{
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: id,
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current == nil || permit.Valid() ||
					scope.ID != id || !scope.Current.Valid() ||
					scope.Successor.Valid() {
					return nil, errors.New("kernel test remove scope differs")
				}
				return &kernelTestRemoveTransaction{
					scope: scope, resource: current,
				}, nil
			},
		},
	}
}

type kernelTestInstallTransaction struct {
	scope    lifecycle.ResourceTransactionScope
	permit   lifecycle.LongLivedPermit
	resource *kernelTestReadyResource
}

func (ktit *kernelTestInstallTransaction) Scope() lifecycle.ResourceTransactionScope {
	return ktit.scope
}

func (ktit *kernelTestInstallTransaction) Apply(
	context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	if err := ktit.permit.ActivateExternal(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	ktit.resource.identity = ktit.scope.Successor
	ktit.resource.permit = ktit.permit
	if err := ktit.resource.Publish(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		ktit.scope,
		lifecycle.ResourceTransactionInstalled,
		ktit.resource,
		result,
		func() error { return nil },
	)
}

func (ktit *kernelTestInstallTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	return nil, ktit.permit.AbortUnused()
}

type kernelTestRemoveTransaction struct {
	scope    lifecycle.ResourceTransactionScope
	resource lifecycle.ReadyResource
}

func (ktrt *kernelTestRemoveTransaction) Scope() lifecycle.ResourceTransactionScope {
	return ktrt.scope
}

func (ktrt *kernelTestRemoveTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	if err := ktrt.resource.Stop(ctx); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if err := ktrt.resource.Finalize(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		ktrt.scope,
		lifecycle.ResourceTransactionRemoved,
		nil,
		result,
		func() error { return nil },
	)
}

func (ktrt *kernelTestRemoveTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	return ktrt.resource, nil
}

func newKernelTestReadyResource(id string, publishRelease, stopRelease <-chan struct{}) *kernelTestReadyResource {
	return &kernelTestReadyResource{
		identity:       lifecycle.ResourceIdentity{ID: id, Generation: 1},
		publishEntered: make(chan struct{}),
		publishRelease: publishRelease,
		stopEntered:    make(chan struct{}),
		stopRelease:    stopRelease,
	}
}

type kernelTestReadyResource struct {
	identity       lifecycle.ResourceIdentity
	permit         lifecycle.LongLivedPermit
	publishEntered chan struct{}
	publishRelease <-chan struct{}
	stopEntered    chan struct{}
	stopRelease    <-chan struct{}
	publishOnce    sync.Once
	stopOnce       sync.Once
}

func (ktrr *kernelTestReadyResource) Identity() lifecycle.ResourceIdentity {
	return ktrr.identity
}

func (ktrr *kernelTestReadyResource) Publish() error {
	ktrr.publishOnce.Do(func() { close(ktrr.publishEntered) })
	if ktrr.publishRelease != nil {
		<-ktrr.publishRelease
	}
	return nil
}

func (ktrr *kernelTestReadyResource) AbortReady(context.Context) error {
	return errors.Join(
		ktrr.permit.ReleaseExternal(),
		ktrr.permit.Return(),
	)
}

func (ktrr *kernelTestReadyResource) Stop(context.Context) error {
	ktrr.stopOnce.Do(func() { close(ktrr.stopEntered) })
	if ktrr.stopRelease != nil {
		<-ktrr.stopRelease
	}
	return ktrr.permit.ReleaseExternal()
}

func (ktrr *kernelTestReadyResource) Finalize() error {
	return ktrr.permit.Return()
}

type stoppedKernelPlanner struct{}

func (stoppedKernelPlanner) Plan(Request) (WorkPlan, error) {
	return WorkPlan{Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})}, nil
}

func startKernelLoop(t *testing.T, kernel *testCommandKernel) {
	t.Helper()
	require.NoError(t, kernel.Start(context.Background()))
}
