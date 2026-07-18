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
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestKernelCompletionBroadcastsToAllCallers(t *testing.T) {
	t.Run("repeat wait", func(t *testing.T) {
		kernel := newStoppedKernel(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := kernel.Wait(ctx); err != nil {
			t.Fatalf("second wait differs: %v", err)
		}
	})

	t.Run("submit after stop", func(t *testing.T) {
		kernel := newStoppedKernel(t)
		catalog := testFunctionCatalogFor(t, kernel)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := kernel.Submit(ctx, Request{UID: "after-stop", Source: lifecycle.SourceFunction, Route: "route"})
		if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "stopped") {
			t.Fatalf("post-stop submit differs: %v", err)
		}
		if catalog.next != 0 || catalog.release != 0 || len(catalog.leases) != 0 {
			t.Fatalf("post-stop submit touched Function catalog: resolves=%d releases=%d active=%d",
				catalog.next, catalog.release, len(catalog.leases))
		}
	})

	t.Run("cancel after stop", func(t *testing.T) {
		kernel := newStoppedKernel(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := kernel.Cancel(ctx, "after-stop")
		if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "stopped") {
			t.Fatalf("post-stop cancel differs: %v", err)
		}
	})
}

func TestKernelLoopStartsExactlyOnce(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T)
	}{
		"nil command kernel": {
			run: func(t *testing.T) {
				if _, err := NewKernelLoop(nil); err == nil {
					t.Fatal("nil command kernel was accepted")
				}
			},
		},
		"nil context": {
			run: func(t *testing.T) {
				kernel, _ := newKernel(t)
				loop, err := NewKernelLoop(kernel)
				if err != nil {
					t.Fatal(err)
				}
				if err := loop.Start(nil); err == nil {
					t.Fatal("nil loop context was accepted")
				}
			},
		},
		"duplicate start": {
			run: func(t *testing.T) {
				kernel, _ := newKernel(t)
				loop, err := NewKernelLoop(kernel)
				if err != nil {
					t.Fatal(err)
				}
				if err := loop.Start(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := loop.Start(context.Background()); err == nil {
					t.Fatal("second loop start was accepted")
				}
				kernel.Stop()
				if err := kernel.Wait(context.Background()); err != nil {
					t.Fatal(err)
				}
			},
		},
		"duplicate wrappers": {
			run: func(t *testing.T) {
				kernel, _ := newKernel(t)
				first, err := NewKernelLoop(kernel)
				if err != nil {
					t.Fatal(err)
				}
				second, err := NewKernelLoop(kernel)
				if err != nil {
					t.Fatal(err)
				}
				if err := first.Start(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := second.Start(context.Background()); err == nil {
					kernel.Stop()
					_ = kernel.Wait(context.Background())
					t.Fatal("second wrapper start was accepted")
				}
				kernel.Stop()
				if err := kernel.Wait(context.Background()); err != nil {
					t.Fatal(err)
				}
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
		call   func(context.Context, *CommandKernel, int) error
	}{
		"command": {
			source: lifecycle.SourceJobManager,
			call: func(ctx context.Context, kernel *CommandKernel, index int) error {
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
			call: func(ctx context.Context, kernel *CommandKernel, index int) error {
				return kernel.Reject(ctx, fmt.Sprintf("terminal-control-%d", index), lifecycle.ControlBadRequest)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel := newStoppedKernel(t)
			for index := 0; index < externalSourceQueueDepth*4; index++ {
				if err := test.call(context.Background(), kernel, index); !errors.Is(err, ErrStopped) {
					t.Fatalf("terminal submission %d differs: %v", index, err)
				}
			}
			if retained := len(kernel.submissions[sourceIndex(test.source)]); retained != 0 {
				t.Fatalf("terminal submissions retained=%d", retained)
			}
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
	if err := kernel.Wait(ctx); !errors.Is(err, want) {
		t.Fatalf("dirty terminal cause differs: %v", err)
	}
	if err := kernel.Wait(ctx); !errors.Is(err, want) {
		t.Fatalf("repeated dirty terminal cause differs: %v", err)
	}
}

func TestKernelResourcePublicationRunsOffLoop(t *testing.T) {
	publishRelease := make(chan struct{})
	resource := newKernelTestReadyResource("resource", publishRelease, nil)
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, kernelResourcePlanner(t, resource, nil, nil))
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{
		UID: "publish-off-loop", LaneKey: "resource", Source: lifecycle.SourceJobManager, Route: "install",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-resource.publishEntered:
	case <-time.After(time.Second):
		t.Fatal("resource publication did not start")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	shutdownErr := kernel.WaitShutdownStarted(ctx)
	cancel()
	close(publishRelease)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if shutdownErr != nil {
		t.Fatalf("resource publication blocked KernelLoop shutdown: %v", shutdownErr)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf("resource publication retained tasks: active=%d pending=%d", tasks.Active(), tasks.Pending())
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelPlanningRunsOutsideLoop(t *testing.T) {
	planEntered := make(chan struct{})
	planRelease := make(chan struct{})
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		close(planEntered)
		<-planRelease
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	kernel, run, admission, uids, _ := newKernelWithPlanner(t, planner)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	submitResult := make(chan error, 1)
	go func() {
		submitResult <- kernel.Submit(context.Background(), Request{
			UID: "blocked-planning", LaneKey: "lane", Source: lifecycle.SourceJobManager, Route: "route",
		})
	}()
	select {
	case <-planEntered:
	case <-time.After(time.Second):
		t.Fatal("planning did not start")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	shutdownErr := kernel.WaitShutdownStarted(ctx)
	cancel()
	close(planRelease)
	select {
	case <-submitResult:
	case <-time.After(time.Second):
		t.Fatal("blocked planning caller did not return")
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if shutdownErr != nil {
		t.Fatalf("plan preparation blocked KernelLoop shutdown: %v", shutdownErr)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestFunctionCatalogPlanningWaitsForKernelLoop(t *testing.T) {
	planned := make(chan struct{}, 1)
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		planned <- struct{}{}
		return WorkPlan{
			Work: lifecycle.FrameTaskWork(plannerPlanWork),
		}, nil
	})
	kernel, run, admission, uids, _ := newKernelWithPlanner(t, planner)
	catalog := testFunctionCatalogFor(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

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
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Function submission did not complete")
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
	if calledBeforeStart {
		t.Fatal("Function catalog planning ran before KernelLoop")
	}
	if catalog.next != 1 || catalog.release != 1 || len(catalog.leases) != 0 {
		t.Fatalf("Function lease lifecycle differs: resolves=%d releases=%d active=%d",
			catalog.next, catalog.release, len(catalog.leases))
	}
}

func TestFunctionCatalogDecisionOwnsExactLease(t *testing.T) {
	work := WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)}
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
			kernel, run, admission, uids, _ := newKernelWithClockFinalizerCatalogAndTimeout(
				t, planner, catalog, &output, lifecycle.RealClock{}, newNoopRunFinalizer(), time.Second,
			)
			if err := run.OpenAdmission(); err != nil {
				t.Fatal(err)
			}
			startKernelLoop(t, kernel)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			err := kernel.SubmitAndWait(ctx, Request{
				UID: "catalog-decision", Source: lifecycle.SourceFunction, Route: "route",
			})
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("SubmitAndWait error=%v, want error=%v", err, test.wantErr)
			}
			kernel.Stop()
			if err := kernel.Wait(context.Background()); err != nil {
				t.Fatal(err)
			}
			if resolves != 1 || releases != test.wantReleases {
				t.Fatalf("catalog transitions: resolves=%d releases=%d, want 1/%d",
					resolves, releases, test.wantReleases)
			}
			if test.wantStatus != "" && !bytes.Contains(output.Bytes(), []byte(test.wantStatus)) {
				t.Fatalf("Function result status differs: %q", output.Bytes())
			}
			if err := admission.CloseDrained(run.Generation()); err != nil {
				t.Fatal(err)
			}
			closeUIDLedger(t, uids)
		})
	}
}

func TestFunctionHandlerCleanupRunsOffKernelLoop(t *testing.T) {
	cleanupCompleted := make(chan error, 1)
	var kernel *CommandKernel
	catalog := functionCatalogPortStub{
		resolve: func(FunctionLookup) (FunctionCatalogDecision, error) {
			return FunctionCatalogDecision{
				Plan:  WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)},
				Lease: FunctionInvocationRef{Slot: 1, Generation: 1},
			}, nil
		},
		release: func(FunctionInvocationRef) (FunctionCleanupPlan, error) {
			return FunctionCleanupPlan{
				Ref: FunctionCleanupRef{Slot: 1, Generation: 1},
				Work: func(ctx context.Context) (lifecycle.TaskOutcome, error) {
					if err := kernel.Cancel(ctx, "cleanup-loop-barrier"); err != nil {
						return lifecycle.NoValueOutcome(), err
					}
					return lifecycle.NoValueOutcome(), nil
				},
			}, nil
		},
		complete: func(_ FunctionCleanupRef, err error) error {
			cleanupCompleted <- err
			return nil
		},
	}
	planner := stoppedKernelPlanner{}
	var run *lifecycle.RunSupervisor
	var admission *lifecycle.AdmissionLedger
	var uids *lifecycle.UIDLedger
	kernel, run, admission, uids, _ = newKernelWithClockFinalizerCatalogAndTimeout(
		t, planner, catalog, io.Discard, lifecycle.RealClock{}, newNoopRunFinalizer(), time.Second,
	)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.SubmitAndWait(context.Background(), Request{
		UID: "cleanup-off-loop", Source: lifecycle.SourceFunction, Route: "route",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-cleanupCompleted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Function cleanup did not complete through TaskSupervisor")
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(kernel.functionCleanupRequests) != 0 || len(kernel.functionCleanupTasks) != 0 {
		t.Fatalf("Function cleanup retained kernel state: requests=%d tasks=%d",
			len(kernel.functionCleanupRequests), len(kernel.functionCleanupTasks))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestWorkPlanRejectsUnboundedClaims(t *testing.T) {
	valid := WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)}
	tests := map[string]struct {
		mutate func(*WorkPlan)
	}{
		"too many claims": {
			mutate: func(plan *WorkPlan) {
				plan.Claims = make([]string, maximumPlanClaims+1)
				for index := range plan.Claims {
					plan.Claims[index] = "claim"
				}
			},
		},
		"oversized claim key": {
			mutate: func(plan *WorkPlan) {
				plan.Claims = []string{strings.Repeat("c", maximumClaimKeyBytes+1)}
			},
		},
		"oversized aggregate claims": {
			mutate: func(plan *WorkPlan) {
				plan.ReadClaims = []string{
					strings.Repeat("a", maximumPlanClaimBytes/2),
					strings.Repeat("b", maximumPlanClaimBytes/2+1),
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plan := valid
			test.mutate(&plan)
			if err := plan.validate(); err == nil {
				t.Fatal("unbounded plan passed validation")
			}
		})
	}
}

func TestKernelSubmitWaitsForOrdinaryAdmissionGrant(t *testing.T) {
	kernel, run, admission, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	gate := make(chan struct{})
	kernel.admissionServiceGate = gate
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	request := Request{
		UID: "grant-boundary", Source: lifecycle.SourceFunction, Route: "route",
	}
	plan, err := kernel.prepareSubmissionPlanForTest(request)
	if err != nil {
		t.Fatal(err)
	}
	submitted := make(chan error, 1)
	kernel.submissions[sourceIndex(request.Source)] <- submission{
		request: request,
		plan:    plan,
		result:  submitted,
	}
	kernel.serviceSubmissions(1)
	if census := admission.Census(); census.OrdinaryWaiting != 1 || census.OrdinaryGranted != 0 {
		t.Fatalf("pre-grant admission differs: %#v", census)
	}
	select {
	case err := <-submitted:
		t.Fatalf("submission returned before ordinary grant: %v", err)
	default:
	}

	close(gate)
	kernel.serviceAdmissions(1)
	select {
	case err := <-submitted:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("submission did not return after ordinary grant")
	}
}

func TestKernelCancelledSubmitReleasesUngrantableAdmission(t *testing.T) {
	kernel, run, admission, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	catalog := testFunctionCatalogFor(t, kernel)
	gate := make(chan struct{})
	kernel.admissionServiceGate = gate
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)

	ctx, cancel := context.WithCancel(context.Background())
	submitted := make(chan error, 1)
	go func() {
		submitted <- kernel.Submit(ctx, Request{
			UID: "cancel-before-grant", Source: lifecycle.SourceFunction, Route: "route",
		})
	}()
	deadline := time.Now().Add(time.Second)
	for admission.Census().OrdinaryWaiting != 1 {
		if time.Now().After(deadline) {
			t.Fatal("submission did not reach ordinary admission")
		}
		runtime.Gosched()
	}
	cancel()
	select {
	case err := <-submitted:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled submission result differs: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled submission did not return")
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.OrdinaryWaiting != 0 {
		t.Fatalf("cancelled submission retained admission: %#v", census)
	}

	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if catalog.next != 1 || catalog.release != 1 || len(catalog.leases) != 0 {
		t.Fatalf("pre-start cancellation Function lease differs: resolves=%d releases=%d active=%d",
			catalog.next, catalog.release, len(catalog.leases))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelAbsentResourceStopSettlesWithoutAdmission(t *testing.T) {
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		return WorkPlan{
			Resource:   &ResourcePlan{Action: ResourceStop, ID: route},
			NoResponse: true,
		}, nil
	})
	kernel, run, admission, uids, _ := newKernelWithPlanner(t, planner)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)

	result := make(chan error, 1)
	go func() {
		result <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "absent-stop", LaneKey: "resource", Source: lifecycle.SourceJobManager, Route: "resource",
		})
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("absent resource stop did not settle")
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.OrdinaryWaiting != 0 || census.OrdinaryGranted != 0 {
		t.Fatalf("absent stop consumed admission: %#v", census)
	}

	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
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
			events := []string{}
			current := newKernelTestReadyResource("resource", nil, nil)
			successor := newKernelTestReadyResource("resource", nil, nil)
			permitPlan, err := lifecycle.NewJobLongLivedPlan(4096)
			if err != nil {
				t.Fatal(err)
			}
			planner := kernelTestTransactionPlanner{
				permitPlan: permitPlan,
				current:    current,
				successor:  successor,
				prepareErr: test.prepareErr,
				events:     &events,
			}
			var output bytes.Buffer
			kernel, run, admission, uids, tasks :=
				newKernelWithPlannerAndWriter(t, planner, &output)
			if err := run.OpenAdmission(); err != nil {
				t.Fatal(err)
			}
			startKernelLoop(t, kernel)

			if err := kernel.SubmitAndWait(
				context.Background(),
				Request{
					UID:     "install-current",
					LaneKey: "resource",
					Source:  lifecycle.SourceJobManager,
					Route:   "install",
				},
			); err != nil {
				t.Fatal(err)
			}
			originalIdentity := current.identity
			uid := "replace-rejected"
			if test.wantSuccessor {
				uid = "replace-success"
			}
			if err := kernel.SubmitAndWait(
				context.Background(),
				Request{
					UID:     uid,
					LaneKey: "resource",
					Source:  lifecycle.SourceJobManager,
					Route:   "replace",
				},
			); err != nil {
				t.Fatal(err)
			}

			lane := kernel.lanes[resourceCommandLaneKey("resource")]
			if lane == nil {
				t.Fatal("transaction released the live resource lane")
			}
			if test.wantSuccessor {
				if lane.current != successor ||
					lane.currentIdentity != successor.identity ||
					lane.currentIdentity == originalIdentity {
					t.Fatalf(
						"replacement current=%T identity=%#v, successor=%T identity=%#v",
						lane.current,
						lane.currentIdentity,
						successor,
						successor.identity,
					)
				}
			} else if lane.current != current ||
				lane.currentIdentity != originalIdentity {
				t.Fatalf(
					"rejected replacement current=%T identity=%#v, want exact original %#v",
					lane.current,
					lane.currentIdentity,
					originalIdentity,
				)
			}
			if lane.currentStopping ||
				lane.retiringIdentity.Valid() ||
				lane.transactionPlanned != 0 {
				t.Fatalf("transaction retained lane state: %#v", lane)
			}
			if !reflect.DeepEqual(events, test.wantEvents) {
				t.Fatalf("events=%v, want %v", events, test.wantEvents)
			}
			if !strings.Contains(output.String(), test.wantResponseText) {
				t.Fatalf(
					"response does not contain UID %q: %q",
					test.wantResponseText,
					output.String(),
				)
			}

			kernel.Stop()
			waitCtx, cancel := context.WithTimeout(
				context.Background(),
				time.Second,
			)
			defer cancel()
			if err := kernel.Wait(waitCtx); err != nil {
				t.Fatal(err)
			}
			if tasks.Active() != 0 || tasks.Pending() != 0 {
				t.Fatalf(
					"transaction retained tasks: active=%d pending=%d",
					tasks.Active(),
					tasks.Pending(),
				)
			}
			if err := admission.CloseDrained(run.Generation()); err != nil {
				t.Fatal(err)
			}
			closeUIDLedger(t, uids)
		})
	}
}

func TestKernelSharesResourceAuthorityAcrossSchedulingSources(t *testing.T) {
	events := []string{}
	current := newKernelTestReadyResource("resource", nil, nil)
	successor := newKernelTestReadyResource("resource", nil, nil)
	permitPlan, err := lifecycle.NewJobLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
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
	kernel, run, admission, uids, tasks :=
		newKernelWithPlannerAndWriter(t, planner, &output)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "resource"
	})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)

	if err := kernel.SubmitAndWait(
		context.Background(),
		Request{
			UID:     "install-from-job-manager",
			LaneKey: "resource",
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := kernel.SubmitAndWait(
		context.Background(),
		Request{
			UID:    "replace-from-function",
			Source: lifecycle.SourceFunction,
			Route:  "replace",
		},
	); err != nil {
		t.Fatal(err)
	}

	lane := kernel.lanes[resourceCommandLaneKey("resource")]
	if lane == nil ||
		lane.current != successor ||
		lane.currentIdentity != successor.identity ||
		lane.resourceSource != lifecycle.SourceFunction {
		t.Fatalf(
			"cross-source replacement did not retain one exact resource authority: %#v",
			lane,
		)
	}
	resourceLanes := 0
	for key := range kernel.lanes {
		if key.resource && key.key == "resource" {
			resourceLanes++
		}
	}
	if resourceLanes != 1 {
		t.Fatalf(
			"cross-source replacement created %d resource authorities, want 1",
			resourceLanes,
		)
	}
	if want := []string{"prepare", "apply", "cleanup"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v, want %v", events, want)
	}
	if !strings.Contains(
		output.String(),
		"FUNCTION_RESULT_BEGIN replace-from-function 200 ",
	) {
		t.Fatalf(
			"cross-source transaction response missing: %q",
			output.String(),
		)
	}

	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"cross-source transaction retained tasks: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelDisposesCancelledPreparedResourceTransaction(t *testing.T) {
	events := []string{}
	current := newKernelTestReadyResource("resource", nil, nil)
	successor := newKernelTestReadyResource("resource", nil, nil)
	permitPlan, err := lifecycle.NewJobLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	planner := kernelTestTransactionPlanner{
		permitPlan:          permitPlan,
		current:             current,
		successor:           successor,
		waitForCancellation: true,
		events:              &events,
	}
	var output bytes.Buffer
	kernel, run, admission, uids, tasks :=
		newKernelWithPlannerAndWriter(t, planner, &output)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.SubmitAndWait(
		context.Background(),
		Request{
			UID:     "install-before-cancel",
			LaneKey: "resource",
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		},
	); err != nil {
		t.Fatal(err)
	}
	originalIdentity := current.identity
	if err := kernel.SubmitAndWait(
		context.Background(),
		Request{
			UID:      "replace-deadline",
			LaneKey:  "resource",
			Source:   lifecycle.SourceJobManager,
			Route:    "replace",
			Deadline: time.Now().Add(10 * time.Millisecond),
		},
	); err != nil {
		t.Fatal(err)
	}
	lane := kernel.lanes[resourceCommandLaneKey("resource")]
	if lane == nil ||
		lane.current != current ||
		lane.currentIdentity != originalIdentity ||
		lane.currentStopping ||
		lane.transactionPlanned != 0 {
		t.Fatalf("cancelled transaction did not restore exact current: %#v", lane)
	}
	if want := []string{"prepare", "dispose"}; !reflect.DeepEqual(
		events,
		want,
	) {
		t.Fatalf("events=%v, want %v", events, want)
	}
	if !strings.Contains(output.String(), "replace-deadline") {
		t.Fatalf("deadline response missing: %q", output.String())
	}

	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"cancelled transaction retained tasks: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelPreparedInternalTransactionAppliesWithoutResponse(t *testing.T) {
	events := []string{}
	current := newKernelTestReadyResource("resource", nil, nil)
	successor := newKernelTestReadyResource("resource", nil, nil)
	permitPlan, err := lifecycle.NewJobLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	planner := kernelTestTransactionPlanner{
		permitPlan: permitPlan,
		current:    current,
		successor:  successor,
		events:     &events,
	}
	var output bytes.Buffer
	kernel, run, admission, uids, tasks :=
		newKernelWithPlannerAndWriter(t, planner, &output)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.SubmitAndWait(
		context.Background(),
		Request{
			UID:     "prepared-install",
			LaneKey: "resource",
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		},
	); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	plan.NoResponse = true
	if err := kernel.SubmitPreparedAndWait(
		context.Background(),
		request,
		plan,
	); err != nil {
		t.Fatal(err)
	}
	lane := kernel.lanes[resourceCommandLaneKey("resource")]
	if lane == nil ||
		lane.current != successor ||
		lane.currentIdentity != successor.identity ||
		lane.currentStopping ||
		lane.transactionPlanned != 0 {
		t.Fatalf("prepared transaction lane=%#v", lane)
	}
	if want := []string{"prepare", "apply", "cleanup"}; !reflect.DeepEqual(
		events,
		want,
	) {
		t.Fatalf("events=%v want=%v", events, want)
	}
	if strings.Contains(output.String(), request.UID) {
		t.Fatalf("internal transaction emitted a response: %q", output.String())
	}

	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"prepared transaction retained tasks: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelShutdownStopsResourceAfterActiveUserDrains(t *testing.T) {
	stopRelease := make(chan struct{})
	workEntered := make(chan struct{})
	workRelease := make(chan struct{})
	resource := newKernelTestReadyResource("resource", nil, stopRelease)
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, kernelResourcePlanner(t, resource, workEntered, workRelease))
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "resource"
	})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.SubmitAndWait(context.Background(), Request{
		UID: "install-before-use", LaneKey: "resource", Source: lifecycle.SourceJobManager, Route: "install",
	}); err != nil {
		t.Fatal(err)
	}
	useResult := make(chan error, 1)
	go func() {
		useResult <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "active-resource-user", Source: lifecycle.SourceFunction, Route: "use",
		})
	}()
	select {
	case <-workEntered:
	case err := <-useResult:
		t.Fatalf("resource user reached terminal before starting: %v", err)
	case <-time.After(time.Second):
		t.Fatal("resource user did not start")
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
		t.Fatal("resource stop did not begin after its user drained")
	}
	close(stopRelease)
	select {
	case <-useResult:
	case <-time.After(time.Second):
		t.Fatal("resource user did not reach terminal disposal")
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if overlap {
		t.Fatal("resource stop overlapped an active same-lane user")
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf("resource shutdown retained tasks: active=%d pending=%d", tasks.Active(), tasks.Pending())
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelShutdownTracksDynamicTaskPopulation(t *testing.T) {
	const population = 9

	stopRelease := make(chan struct{})
	resources := make(map[string]*kernelTestReadyResource, population)
	for index := 0; index < population; index++ {
		id := fmt.Sprintf("resource-%02d", index)
		resources[id] = newKernelTestReadyResource(id, nil, stopRelease)
	}
	permitPlan, err := lifecycle.NewJobLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
	}
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, kernelTestResourceSetPlanner{
		permitPlan: permitPlan,
		resources:  resources,
	})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	for id := range resources {
		if err := kernel.SubmitAndWait(context.Background(), Request{
			UID:     "install-" + id,
			LaneKey: id,
			Source:  lifecycle.SourceJobManager,
			Route:   "install",
		}); err != nil {
			t.Fatal(err)
		}
	}

	kernel.Stop()
	for id, resource := range resources {
		select {
		case <-resource.stopEntered:
		case <-time.After(time.Second):
			t.Fatalf("resource %q did not begin shutdown", id)
		}
	}
	if active := tasks.Active(); active != population {
		t.Fatalf("active shutdown tasks=%d, want %d", active, population)
	}
	wantLongLivedBytes := int64(population) *
		(512 + lifecycle.TaskChildExecutionBytes)
	if census := tasks.LongLivedCensus(); census.Active != population ||
		census.Bytes != wantLongLivedBytes {
		t.Fatalf(
			"active shutdown long-lived census=%+v, want bytes=%d",
			census,
			wantLongLivedBytes,
		)
	}
	if census := admission.Census(); census.LongLivedRecords != population ||
		census.LongLivedBytes != wantLongLivedBytes {
		t.Fatalf(
			"active shutdown admission census=%+v, want bytes=%d",
			census,
			wantLongLivedBytes,
		)
	}
	close(stopRelease)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf("resource shutdown retained tasks: active=%d pending=%d", tasks.Active(), tasks.Pending())
	}
	if census := tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("resource shutdown retained long-lived ownership: %+v", census)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelLoopContinuesPendingTaskStartsAcrossServiceQuanta(t *testing.T) {
	const genericPopulation = lifecycle.TaskStartServiceQuantum * 2

	kernel, run, admission, uids, tasks := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		kernel.Stop()
		waitCtx, waitCancel := context.WithTimeout(
			context.Background(),
			time.Second,
		)
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
		if err != nil {
			t.Fatal(err)
		}
		kernel.functionCleanupRequests[request] = FunctionCleanupRef{
			Slot: slot, Generation: 1,
		}
	}
	for index := 0; index < genericPopulation; index++ {
		enqueueCleanup(uint32(index + 1))
	}

	startKernelLoop(t, kernel)
	for index := 0; index < genericPopulation; index++ {
		select {
		case <-genericEntered:
		case <-time.After(time.Second):
			t.Fatalf(
				"generic task %d remained pending after a service quantum",
				index+1,
			)
		}
	}

	controlEntered := make(chan struct{}, 1)
	if err := kernel.SubmitPrepared(
		context.Background(),
		Request{
			UID:     "continued-framework-control",
			LaneKey: "framework-control",
			Source:  lifecycle.SourceJobManager,
			Route:   "control",
		},
		WorkPlan{
			Work: lifecycle.FrameTaskWork(
				func(context.Context) (lifecycle.SealedResult, error) {
					controlEntered <- struct{}{}
					<-release
					return lifecycle.NewSealedResult(
						200,
						"application/json",
						[]byte(`{}`),
					)
				},
			),
		},
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-controlEntered:
	case <-time.After(time.Second):
		t.Fatal("newly runnable framework-control task did not start")
	}

	releaseOnce.Do(func() { close(release) })
	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"continued task service retained work: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelShutdownCancelsInitialOperationSweepBeforePendingTaskDispatch(
	t *testing.T,
) {
	const population = lifecycle.TaskStartServiceQuantum * 2

	kernel, run, admission, uids, tasks := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		kernel.Stop()
		waitCtx, waitCancel := context.WithTimeout(
			context.Background(),
			time.Second,
		)
		defer waitCancel()
		_ = kernel.Wait(waitCtx)
	})
	entered := make(chan string, population)
	for index := 0; index < population; index++ {
		uid := fmt.Sprintf("shutdown-fence-%02d", index)
		plan := WorkPlan{
			NoResponse: true,
			Work: lifecycle.FrameTaskWork(
				func(context.Context) (lifecycle.SealedResult, error) {
					entered <- uid
					<-release
					return lifecycle.NewSealedResult(
						200,
						"application/json",
						[]byte(`{}`),
					)
				},
			),
		}
		if err := kernel.admit(
			Request{
				UID: uid, LaneKey: uid,
				Source: lifecycle.SourceJobManager,
			},
			plan,
			context.Background(),
			nil,
			nil,
		); err != nil {
			t.Fatal(err)
		}
	}
	for kernel.serviceAdmissions(lifecycle.TaskStartServiceQuantum) {
	}
	for kernel.scheduleTasks(lifecycle.TaskStartServiceQuantum) {
	}
	if tasks.Pending() != population {
		t.Fatalf(
			"shutdown-fence setup pending tasks=%d, want %d",
			tasks.Pending(),
			population,
		)
	}

	kernel.Stop()
	startKernelLoop(t, kernel)
	for index := 0; index < lifecycle.TaskStartServiceQuantum; index++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatalf("initial task %d did not start", index+1)
		}
	}
	select {
	case uid := <-entered:
		t.Fatalf("pending operation %q started after shutdown began", uid)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"shutdown fence retained tasks: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelRunFinalizerUsesSharedBudgetExactlyOnce(t *testing.T) {
	called := make(chan struct {
		generation uint64
		deadline   time.Time
	}, 1)
	finalizer := RunFinalizerFunc(func(ctx context.Context, generation uint64) error {
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
	kernel, run, admission, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	budget, err := run.BeginShutdown()
	if err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	call := <-called
	if call.generation != run.Generation() || !call.deadline.Equal(budget.Deadline()) {
		t.Fatalf("finalizer budget differs: generation=%d deadline=%s want_generation=%d want_deadline=%s", call.generation, call.deadline, run.Generation(), budget.Deadline())
	}
	select {
	case duplicate := <-called:
		t.Fatalf("finalizer ran more than once: %+v", duplicate)
	default:
	}
	if terminal := run.TerminalState(); !terminal.Reached || !terminal.Quiescent || terminal.Dirty != nil {
		t.Fatalf("finalizer terminal differs: %+v", terminal)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || !admission.RunDrained(run.Generation()) {
		t.Fatalf("finalizer left state: active=%d pending=%d admission=%+v", tasks.Active(), tasks.Pending(), admission.Census())
	}
}

func TestKernelShutdownDeadlineWinsFinalizerCompletion(t *testing.T) {
	const helperEnv = "NETDATA_JOBMGR_FINALIZER_DEADLINE_HELPER"
	if os.Getenv(helperEnv) != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestKernelShutdownDeadlineWinsFinalizerCompletion$")
		cmd.Env = append(os.Environ(), helperEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("finalizer-deadline helper failed: %v\n%s", err, output)
		}
		return
	}
	clock := newKernelFinalizerClock()
	started := make(chan struct{})
	finalizer := RunFinalizerFunc(func(ctx context.Context, _ uint64) error {
		close(started)
		<-ctx.Done()
		return nil
	})
	kernel, run, _, _, _ := newKernelWithClockFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, clock, finalizer, time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("finalizer did not start before shutdown expiry")
	}
	clock.expireShutdown(t)
	if err := kernel.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), "shutdown deadline exceeded") {
		t.Fatalf("shutdown expiry lost to finalizer completion: %v", err)
	}
	terminal := run.TerminalState()
	if !terminal.Reached || terminal.Quiescent || terminal.Dirty == nil {
		t.Fatalf("expired finalizer terminal differs: %+v", terminal)
	}
}

func TestKernelDueClockWinsIndependentFinalizerCompletion(t *testing.T) {
	const helperEnv = "NETDATA_JOBMGR_FINALIZER_DUE_CLOCK_HELPER"
	if os.Getenv(helperEnv) != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestKernelDueClockWinsIndependentFinalizerCompletion$")
		cmd.Env = append(os.Environ(), helperEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("due-clock finalizer helper failed: %v\n%s", err, output)
		}
		return
	}
	clock := newKernelFinalizerClock()
	started := make(chan struct{})
	release := make(chan struct{})
	finalizer := RunFinalizerFunc(func(context.Context, uint64) error {
		close(started)
		<-release
		return nil
	})
	kernel, run, _, _, _ := newKernelWithClockFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, clock, finalizer, time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("finalizer did not start before shutdown expiry")
	}
	clock.advanceShutdownWithoutSignal(t)
	close(release)
	if err := kernel.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), "shutdown deadline exceeded") {
		t.Fatalf("due Clock lost to independent finalizer completion: %v", err)
	}
	terminal := run.TerminalState()
	if !terminal.Reached || terminal.Quiescent || terminal.Dirty == nil {
		t.Fatalf("due-clock finalizer terminal differs: %+v", terminal)
	}
}

func TestKernelDueClockDisposesLatePreparedCapabilityWithoutTimerDelivery(t *testing.T) {
	clock := newKernelFinalizerClock()
	prepared := make(chan context.Context, 1)
	release := make(chan struct{})
	permitPlan, err := lifecycle.NewSecretStoreLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	var capability *latePreparedCapability
	const capabilityID = "secret-store:late"
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			NoResponse: true, CooperativeDeadline: true,
			Capability: &CapabilityPlan{
				ID: capabilityID, Permit: permitPlan,
				Prepare: func(ctx context.Context, generation uint64, permit lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
					if err := permit.ActivateExternal(lifecycle.LongLivedESecretStore); err != nil {
						return nil, err
					}
					capability = &latePreparedCapability{
						identity: lifecycle.ResourceIdentity{ID: capabilityID, Generation: generation}, permit: permit,
					}
					prepared <- ctx
					<-release
					return capability, nil
				},
			},
		}, nil
	})
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, newNoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	request := Request{
		UID: "late-capability", LaneKey: capabilityID, Source: lifecycle.SourceJobManager, Route: "late",
		Deadline: clock.Now().Add(time.Second),
	}
	done := make(chan error, 1)
	go func() { done <- kernel.SubmitAndWait(context.Background(), request) }()
	var workCtx context.Context
	select {
	case workCtx = <-prepared:
	case <-time.After(time.Second):
		t.Fatal("prepared capability did not enter")
	}
	select {
	case <-workCtx.Done():
		t.Fatalf("host clock canceled prepared capability before authoritative Clock advanced: %v", workCtx.Err())
	default:
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	select {
	case <-workCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("authoritative Clock deadline did not cancel prepared capability")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("late capability terminal result differs: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("late capability did not reach terminal ownership")
	}
	if capability.committed.Load() || !capability.disposed.Load() {
		t.Fatalf("late capability action differs: committed=%v disposed=%v", capability.committed.Load(), capability.disposed.Load())
	}
	if census := tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("late capability retained permit: %+v", census)
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelKeepsUnchangedDeadlineTimerAcrossUnrelatedEvents(t *testing.T) {
	clock := newKernelFinalizerClock()
	workEntered := make(chan struct{})
	releaseWork := make(chan struct{})
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
			close(workEntered)
			<-releaseWork
			return plannerPlanWork(ctx)
		})}, nil
	})
	kernel, run, admission, uids, _ := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, newNoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	done := make(chan error, 1)
	go func() {
		done <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "one-deadline-timer", Source: lifecycle.SourceFunction, Route: "timer",
			Deadline: clock.Now().Add(time.Hour),
		})
	}()
	select {
	case <-workEntered:
	case <-time.After(time.Second):
		t.Fatal("deadline-timer work did not enter")
	}
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		t.Fatal("Kernel did not arm the deadline timer")
	}
	for index := 0; index < 32; index++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := kernel.Cancel(ctx, fmt.Sprintf("absent-%d", index))
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
	if arms := clock.deadlineArmCount(); arms != 1 {
		t.Fatalf("unchanged deadline timer arms=%d want=1", arms)
	}
	close(releaseWork)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline-timer operation did not finish")
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelDeadlineCancelsPendingCapabilityCommit(t *testing.T) {
	clock := newKernelFinalizerClock()
	commitEntered := make(chan context.Context, 1)
	permitPlan, err := lifecycle.NewSecretStoreLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	const capabilityID = "secret-store:commit-deadline"
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			NoResponse: true, CooperativeDeadline: true,
			Capability: &CapabilityPlan{
				ID: capabilityID, Permit: permitPlan,
				Prepare: func(_ context.Context, generation uint64, permit lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
					if err := permit.ActivateExternal(lifecycle.LongLivedESecretStore); err != nil {
						return nil, err
					}
					return &deadlineCommitCapability{
						latePreparedCapability: latePreparedCapability{
							identity: lifecycle.ResourceIdentity{ID: capabilityID, Generation: generation}, permit: permit,
						},
						entered: commitEntered,
					}, nil
				},
			},
		}, nil
	})
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, newNoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	done := make(chan error, 1)
	deadline := clock.Now().Add(time.Second)
	go func() {
		done <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "commit-deadline", LaneKey: capabilityID, Source: lifecycle.SourceJobManager, Route: "commit-deadline",
			Deadline: deadline,
		})
	}()
	var commitCtx context.Context
	select {
	case commitCtx = <-commitEntered:
	case <-time.After(time.Second):
		t.Fatal("capability commit did not enter")
	}
	if got, ok := commitCtx.Deadline(); !ok || !got.Equal(deadline) {
		t.Fatalf("capability commit deadline=%s ok=%v, want %s", got, ok, deadline)
	}
	select {
	case <-commitCtx.Done():
		t.Fatalf("capability commit context canceled before authoritative deadline: %v", commitCtx.Err())
	default:
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	select {
	case <-commitCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("pending capability commit did not observe authoritative deadline")
	}
	if !errors.Is(commitCtx.Err(), context.DeadlineExceeded) || !errors.Is(context.Cause(commitCtx), context.DeadlineExceeded) {
		t.Fatalf("capability commit cancellation err=%v cause=%v", commitCtx.Err(), context.Cause(commitCtx))
	}
	select {
	case terminalErr := <-done:
		if terminalErr == nil || !errors.Is(terminalErr, context.DeadlineExceeded) {
			t.Fatalf("capability commit terminal result differs: %v", terminalErr)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline-canceled capability did not reach terminal ownership")
	}
	if err := kernel.Wait(context.Background()); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline-canceled Kernel result differs: %v", err)
	}
	if census := tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("deadline-canceled commit retained permit: %+v", census)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
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
				Work: lifecycle.FrameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					deadlineCalls.Add(1)
					deadline, ok := ctx.Deadline()
					deadlineEntered <- deadlineObservation{deadline: deadline, ok: ok, err: ctx.Err(), cause: context.Cause(ctx)}
					return lifecycle.NewControlResult(lifecycle.ControlDeadline)
				}),
			}, nil
		}
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			blockerEntered <- struct{}{}
			<-release
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "queued-deadline"
	})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	future := clock.Now().Add(time.Minute)
	if err := kernel.Submit(context.Background(), Request{
		UID: "blocker", Source: lifecycle.SourceFunction,
		Route: "blocker", Deadline: future,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("same-lane blocker did not start")
	}
	due := clock.Now()
	if err := kernel.Submit(context.Background(), Request{
		UID: "queued-deadline", Source: lifecycle.SourceFunction,
		Route: "deadline", Deadline: due,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case observed := <-deadlineEntered:
		t.Fatalf("queued deadline handler bypassed its active lane: %+v", observed)
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
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("queued cooperative deadline handler was terminalized without execution")
	}
	if calls := deadlineCalls.Load(); calls != 1 || !observed.ok || !observed.deadline.Equal(due) ||
		!errors.Is(observed.err, context.DeadlineExceeded) || !errors.Is(observed.cause, context.DeadlineExceeded) {
		t.Fatalf("queued deadline observation calls=%d deadline=%s ok=%v err=%v cause=%v", calls, observed.deadline, observed.ok, observed.err, observed.cause)
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN queued-deadline 504 application/json ")) {
		t.Fatalf("queued deadline response differs: %q", output.Bytes())
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("queued deadline retained state: active=%d pending=%d operations=%d lanes=%d", tasks.Active(), tasks.Pending(), len(kernel.operations), len(kernel.lanes))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
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
				Runner:              runner,
				CooperativeDeadline: true,
			}, nil
		}
		return WorkPlan{
			Work: lifecycle.FrameTaskWork(func(
				context.Context,
			) (lifecycle.SealedResult, error) {
				blockerEntered <- struct{}{}
				<-release
				return lifecycle.NewSealedResult(
					200,
					"application/json",
					[]byte(`{}`),
				)
			}),
		}, nil
	})
	var output bytes.Buffer
	kernel, run, admission, uids, tasks :=
		newKernelWithClockFinalizerAndTimeout(
			t,
			planner,
			&output,
			clock,
			newNoopRunFinalizer(),
			time.Second,
		)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	setTestFunctionResource(t, kernel, func(lookup FunctionLookup) string {
		return "due-runner"
	})
	startKernelLoop(t, kernel)
	if err := kernel.Submit(
		context.Background(),
		Request{
			UID:      "runner-blocker",
			Source:   lifecycle.SourceFunction,
			Route:    "blocker",
			Deadline: clock.Now().Add(time.Minute),
		},
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("same-lane runner blocker did not start")
	}
	result := make(chan error, 1)
	go func() {
		result <- kernel.SubmitAndWait(
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
		t.Fatalf("due cooperative runner bypassed its active lane: %v", cause)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case cause := <-observed:
		if !errors.Is(cause, context.DeadlineExceeded) {
			t.Fatalf("runner cancellation cause=%v", cause)
		}
	case <-time.After(time.Second):
		t.Fatal("due cooperative runner was terminalized without execution")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("due cooperative runner did not reach terminal disposition")
	}
	if !bytes.Contains(
		output.Bytes(),
		[]byte("FUNCTION_RESULT_BEGIN due-runner 504 application/json "),
	) {
		t.Fatalf("due cooperative runner response differs: %q", output.Bytes())
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf(
			"due cooperative runner retained tasks: active=%d pending=%d",
			tasks.Active(),
			tasks.Pending(),
		)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

type kernelDeadlineRunner struct {
	observed chan<- error
}

func (runner kernelDeadlineRunner) RunTask(
	ctx context.Context,
) (lifecycle.TaskOutcome, error) {
	runner.observed <- context.Cause(ctx)
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
				Work: lifecycle.FrameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					deadlineCalls.Add(1)
					deadline, ok := ctx.Deadline()
					deadlineEntered <- deadlineObservation{deadline: deadline, ok: ok, err: ctx.Err(), cause: context.Cause(ctx)}
					return lifecycle.NewControlResult(lifecycle.ControlDeadline)
				}),
			}, nil
		}
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			predecessorEntered <- struct{}{}
			<-releasePredecessor
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string { return "same-lane" })
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	predecessorResult := make(chan error, 1)
	go func() {
		predecessorResult <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "same-lane-predecessor", Source: lifecycle.SourceFunction, Route: "predecessor",
		})
	}()
	select {
	case <-predecessorEntered:
	case <-time.After(time.Second):
		t.Fatal("same-lane predecessor did not start")
	}
	due := clock.Now().Add(time.Second)
	deadlineResult := make(chan error, 1)
	go func() {
		deadlineResult <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "same-lane-deadline", Source: lifecycle.SourceFunction,
			Route: "deadline", Deadline: due,
		})
	}()
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		t.Fatal("same-lane successor deadline was not armed")
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	barrierCtx, barrierCancel := context.WithTimeout(context.Background(), time.Second)
	defer barrierCancel()
	if err := kernel.Cancel(barrierCtx, "same-lane-deadline"); err != nil {
		t.Fatal(err)
	}
	select {
	case observed := <-deadlineEntered:
		t.Fatalf("same-lane deadline handler bypassed its predecessor: %+v", observed)
	default:
	}
	close(releasePredecessor)
	select {
	case err := <-predecessorResult:
		if err != nil {
			t.Fatalf("same-lane predecessor result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("same-lane predecessor did not complete")
	}
	var observed deadlineObservation
	select {
	case observed = <-deadlineEntered:
	case <-time.After(time.Second):
		t.Fatal("expired same-lane successor was not scheduled")
	}
	select {
	case err := <-deadlineResult:
		if err != nil {
			t.Fatalf("same-lane deadline result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("same-lane deadline operation did not complete")
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := deadlineCalls.Load(); calls != 1 || !observed.ok || !observed.deadline.Equal(due) ||
		!errors.Is(observed.err, context.DeadlineExceeded) || !errors.Is(observed.cause, context.DeadlineExceeded) {
		t.Fatalf("same-lane deadline observation calls=%d deadline=%s ok=%v err=%v cause=%v", calls, observed.deadline, observed.ok, observed.err, observed.cause)
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN same-lane-deadline 504 application/json ")) {
		t.Fatalf("same-lane deadline response differs: %q", output.Bytes())
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("same-lane deadline retained state: active=%d pending=%d operations=%d lanes=%d", tasks.Active(), tasks.Pending(), len(kernel.operations), len(kernel.lanes))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelDisposesQueuedNoResponseCapabilityAfterItsDeadline(t *testing.T) {
	clock := newKernelFinalizerClock()
	releaseBlockers := make(chan struct{})
	blockerEntered := make(chan struct{}, 1)
	permitPlan, err := lifecycle.NewSecretStoreLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	var prepareCalls atomic.Int32
	const capabilityID = "secret-store:queued-deadline"
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "capability" {
			return WorkPlan{
				NoResponse: true,
				Capability: &CapabilityPlan{
					ID: capabilityID, Permit: permitPlan,
					Prepare: func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
						prepareCalls.Add(1)
						return nil, errors.New("queued capability unexpectedly started")
					},
				},
			}, nil
		}
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			blockerEntered <- struct{}{}
			<-releaseBlockers
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, newNoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{
		UID: "queued-capability-blocker", LaneKey: capabilityID,
		Source: lifecycle.SourceJobManager, Route: "blocker",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("same-lane capability blocker did not start")
	}
	terminal := make(chan error, 1)
	go func() {
		terminal <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "queued-no-response-capability", LaneKey: capabilityID, Source: lifecycle.SourceJobManager,
			Route: "capability", Deadline: clock.Now(),
		})
	}()
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		close(releaseBlockers)
		kernel.Stop()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = kernel.Wait(cleanupCtx)
		t.Fatal("queued no-response capability deadline was not armed")
	}
	kernel.NotifyControlReady()
	select {
	case err := <-terminal:
		if err != nil {
			t.Fatalf("queued no-response capability terminal result differs: %v", err)
		}
	case <-time.After(time.Second):
		close(releaseBlockers)
		kernel.Stop()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = kernel.Wait(cleanupCtx)
		t.Fatal("queued no-response capability did not reach terminal disposal")
	}
	if prepareCalls.Load() != 0 {
		t.Fatalf("queued no-response capability prepare=%d, want 0", prepareCalls.Load())
	}
	close(releaseBlockers)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("queued no-response capability retained state: active=%d pending=%d operations=%d lanes=%d", tasks.Active(), tasks.Pending(), len(kernel.operations), len(kernel.lanes))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
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
				Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
					workCalls.Add(1)
					return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
				}),
			}, nil
		}
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			blockerEntered <- struct{}{}
			<-releaseBlockers
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string {
		return "queued-noncooperative"
	})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{
		UID:    "noncooperative-blocker",
		Source: lifecycle.SourceFunction, Route: "blocker",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("same-lane noncooperative blocker did not start")
	}
	terminal := make(chan error, 1)
	go func() {
		terminal <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "queued-noncooperative-deadline", Source: lifecycle.SourceFunction,
			Route: "noncooperative", Deadline: clock.Now(),
		})
	}()
	select {
	case <-clock.deadlineArmed:
	case <-time.After(time.Second):
		t.Fatal("queued noncooperative deadline was not armed")
	}
	kernel.NotifyControlReady()
	select {
	case err := <-terminal:
		if err != nil {
			t.Fatalf("queued noncooperative deadline result differs: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued noncooperative deadline did not reach terminal disposal")
	}
	if workCalls.Load() != 0 {
		t.Fatalf("queued noncooperative work calls=%d, want 0", workCalls.Load())
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN queued-noncooperative-deadline 504 application/json ")) {
		t.Fatalf("queued noncooperative deadline response differs: %q", output.Bytes())
	}
	close(releaseBlockers)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("queued noncooperative deadline retained state: active=%d pending=%d operations=%d lanes=%d", tasks.Active(), tasks.Pending(), len(kernel.operations), len(kernel.lanes))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelOneRetainedTimeoutPlusThreeActiveTasksDoesNotDirty(t *testing.T) {
	clock := newKernelFinalizerClock()
	entered := make(chan string, lifecycle.RetainedTimeoutFailStopThreshold)
	releaseWork := make(chan struct{})
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			entered <- route
			<-releaseWork
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	writer := &firstHoldingFrameWriter{offered: make(chan []byte, 1), release: make(chan struct{})}
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, writer, clock, newNoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	deadline := clock.Now().Add(time.Second)
	terminals := make([]chan error, lifecycle.RetainedTimeoutFailStopThreshold)
	for index := 0; index < lifecycle.RetainedTimeoutFailStopThreshold; index++ {
		terminals[index] = make(chan error, 1)
		request := Request{
			UID:    fmt.Sprintf("mixed-retained-%d", index),
			Source: lifecycle.SourceFunction, Route: fmt.Sprintf("work-%d", index),
		}
		if index == 0 {
			request.Deadline = deadline
		}
		if err := kernel.submit(context.Background(), request, terminals[index]); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.RetainedTimeoutFailStopThreshold; index++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("mixed retained TaskChild did not start")
		}
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	select {
	case frame := <-writer.offered:
		if !bytes.Contains(frame, []byte("FUNCTION_RESULT_BEGIN mixed-retained-0 504 application/json ")) {
			t.Fatalf("mixed retained timeout frame differs: %q", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("mixed retained timeout frame was not offered")
	}
	close(writer.release)
	barrierCtx, barrierCancel := context.WithTimeout(context.Background(), time.Second)
	defer barrierCancel()
	if err := kernel.Cancel(barrierCtx, "retained-count-barrier"); err != nil {
		t.Fatal(err)
	}
	if count, saturated := tasks.RetainedTimeouts(); count != 1 || saturated {
		t.Fatalf("mixed retained timeout census=(%d,%v), want (1,false)", count, saturated)
	}
	if err := run.DirtyCause(); err != nil {
		t.Fatalf("one retained timeout plus three unrelated active tasks dirtied the run: %v", err)
	}
	close(releaseWork)
	for index, terminal := range terminals {
		select {
		case err := <-terminal:
			if err != nil {
				t.Fatalf("mixed retained terminal %d: %v", index, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("mixed retained terminal %d did not complete", index)
		}
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count, saturated := tasks.RetainedTimeouts(); count != 0 || saturated {
		t.Fatalf("completed mixed retained census=(%d,%v), want (0,false)", count, saturated)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("mixed retained test left state: active=%d pending=%d operations=%d lanes=%d", tasks.Active(), tasks.Pending(), len(kernel.operations), len(kernel.lanes))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelFourthBackgroundTimeoutDirtiesWithoutResponseCommit(t *testing.T) {
	clock := newKernelFinalizerClock()
	release := make(chan struct{})
	entered := make(chan string, lifecycle.RetainedTimeoutFailStopThreshold)
	permitPlan, err := lifecycle.NewJobLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	planner := plannerFunc(func(_ context.Context, route string, args []string) (WorkPlan, error) {
		if route != "background-capability" || len(args) != 1 {
			return WorkPlan{}, errors.New("unexpected background capability request")
		}
		id := args[0]
		return WorkPlan{
			NoResponse: true, CooperativeDeadline: true,
			Capability: &CapabilityPlan{
				ID: id, Permit: permitPlan,
				Prepare: func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
					entered <- id
					<-release
					return nil, nil
				},
			},
		}, nil
	})
	var output bytes.Buffer
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, newNoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	deadline := clock.Now().Add(time.Second)
	terminals := make([]chan error, lifecycle.RetainedTimeoutFailStopThreshold)
	for index := 0; index < lifecycle.RetainedTimeoutFailStopThreshold; index++ {
		id := fmt.Sprintf("job:background-%d", index)
		terminals[index] = make(chan error, 1)
		if err := kernel.submit(context.Background(), Request{
			UID: fmt.Sprintf("background-timeout-%d", index), LaneKey: id, Source: lifecycle.SourceJobManager,
			Route: "background-capability", Args: []string{id}, Deadline: deadline,
		}, terminals[index]); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.RetainedTimeoutFailStopThreshold; index++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("background capability did not occupy its TaskChild slot")
		}
	}
	clock.advance(time.Second + time.Nanosecond)
	kernel.NotifyControlReady()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := kernel.WaitShutdownStarted(shutdownCtx); err != nil {
		t.Fatalf("fourth background timeout did not start dirty shutdown: %v", err)
	}
	if cause := run.DirtyCause(); cause == nil || !strings.Contains(cause.Error(), "fourth background timeout reached the retained-timeout fail-stop threshold") {
		t.Fatalf("fourth background timeout dirty cause differs: %v", cause)
	}
	if count, saturated := tasks.RetainedTimeouts(); count != lifecycle.RetainedTimeoutFailStopThreshold || !saturated {
		t.Fatalf("background timeout census=(%d,%v), want (%d,true)", count, saturated, lifecycle.RetainedTimeoutFailStopThreshold)
	}
	close(release)
	for index, terminal := range terminals {
		select {
		case err := <-terminal:
			if err != nil {
				t.Fatalf("background terminal %d: %v", index, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("background terminal %d did not complete", index)
		}
	}
	terminalErr := kernel.Wait(context.Background())
	if terminalErr == nil || !strings.Contains(terminalErr.Error(), "fourth background timeout reached the retained-timeout fail-stop threshold") {
		t.Fatalf("background timeout terminal error differs: %v", terminalErr)
	}
	if output.Len() != 0 {
		t.Fatalf("background timeout emitted a response: %q", output.Bytes())
	}
	if count, saturated := tasks.RetainedTimeouts(); count != 0 || !saturated {
		t.Fatalf("completed background timeout census=(%d,%v), want (0,true)", count, saturated)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 || tasks.LongLivedCensus() != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("background timeout retained state: active=%d pending=%d operations=%d lanes=%d long_lived=%+v", tasks.Active(), tasks.Pending(), len(kernel.operations), len(kernel.lanes), tasks.LongLivedCensus())
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

type latePreparedCapability struct {
	identity   lifecycle.ResourceIdentity
	permit     lifecycle.LongLivedPermit
	committed  atomic.Bool
	disposed   atomic.Bool
	once       sync.Once
	releaseErr error
}

type deadlineCommitCapability struct {
	latePreparedCapability
	entered chan<- context.Context
}

func (capability *deadlineCommitCapability) Commit(ctx context.Context, _ uint64) (lifecycle.CapabilityDisposition, error) {
	capability.entered <- ctx
	<-ctx.Done()
	return lifecycle.CapabilityDisposed, errors.Join(ctx.Err(), capability.release())
}

func (capability *latePreparedCapability) Identity() lifecycle.ResourceIdentity {
	return capability.identity
}

func (capability *latePreparedCapability) Commit(context.Context, uint64) (lifecycle.CapabilityDisposition, error) {
	capability.committed.Store(true)
	return lifecycle.CapabilityApplied, capability.release()
}

func (capability *latePreparedCapability) Dispose(context.Context) error {
	capability.disposed.Store(true)
	return capability.release()
}

func (capability *latePreparedCapability) release() error {
	capability.once.Do(func() {
		capability.releaseErr = errors.Join(
			capability.permit.ReleaseExternal(lifecycle.LongLivedESecretStore),
			capability.permit.ReleaseBytes(),
			capability.permit.Return(),
		)
	})
	return capability.releaseErr
}

func TestKernelRunFinalizerReleasesOnlyTypedFinalizerOwnedPermit(t *testing.T) {
	var permit lifecycle.LongLivedPermit
	called := make(chan struct{}, 1)
	finalizer := RunFinalizerFunc(func(context.Context, uint64) error {
		called <- struct{}{}
		if err := permit.ReleaseExternal(lifecycle.LongLivedESecretStore); err != nil {
			return err
		}
		if err := permit.ReleaseBytes(); err != nil {
			return err
		}
		return permit.Return()
	})
	kernel, run, admission, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)
	plan, err := lifecycle.NewSecretStoreLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
	}
	requested := admission.RequestOrdinary(
		run.Generation(),
		lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1},
		plan.Bytes()+512,
	)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	permit, err = tasks.IssueLongLivedPermit(admission, requested.Ref, lifecycle.ResourceIdentity{ID: "secret-store", Generation: 1}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.ActivateExternal(lifecycle.LongLivedESecretStore); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(requested.Ref); err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	default:
		t.Fatal("typed finalizer-owned permit prevented finalizer dispatch")
	}
	if census := tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("finalizer-owned permit remained: %+v", census)
	}
	if terminal := run.TerminalState(); !terminal.Quiescent || terminal.Dirty != nil {
		t.Fatalf("typed finalizer-owned terminal differs: %+v", terminal)
	}
}

func TestKernelLongLivedBoundaryAllowsReplacementAndRejectsSteadyAddition(t *testing.T) {
	var seeded []lifecycle.LongLivedPermit
	finalizer := RunFinalizerFunc(func(context.Context, uint64) error {
		var result error
		for _, permit := range seeded {
			result = errors.Join(result, permit.AbortUnused())
		}
		return result
	})
	steadyPlan, err := lifecycle.NewSecretStoreLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
	replacementPlan, err := lifecycle.NewSecretStoreReplacementLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
	var replacementPrepared atomic.Int32
	var additionPrepared atomic.Int32
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		permitPlan := steadyPlan
		prepared := &additionPrepared
		if route == "replacement" {
			permitPlan = replacementPlan
			prepared = &replacementPrepared
		} else if route != "addition" {
			return WorkPlan{}, errors.New("unexpected long-lived boundary route")
		}
		return WorkPlan{
			NoResponse: true,
			Capability: &CapabilityPlan{
				ID: "secret-store:" + route, Permit: permitPlan,
				Prepare: func(_ context.Context, generation uint64, permit lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
					prepared.Add(1)
					if err := permit.ActivateExternal(lifecycle.LongLivedESecretStore); err != nil {
						return nil, err
					}
					return &latePreparedCapability{
						identity: lifecycle.ResourceIdentity{ID: "secret-store:" + route, Generation: generation}, permit: permit,
					}, nil
				},
			},
		}, nil
	})
	kernel, run, admission, uids, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, planner, io.Discard, finalizer, time.Second)
	seeded = make([]lifecycle.LongLivedPermit, 0, 1)
	requested := admission.RequestOrdinary(
		run.Generation(),
		lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1},
		steadyPlan.Bytes()+1,
	)
	if requested.Rejected != nil {
		t.Fatalf("seed admission: %v", requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf(
			"seed grant differs: count=%d grant=%+v err=%v",
			count,
			grants[0],
			err,
		)
	}
	permit, err := tasks.IssueLongLivedPermit(
		admission,
		requested.Ref,
		lifecycle.ResourceIdentity{ID: "seed", Generation: 1},
		steadyPlan,
	)
	if err != nil {
		t.Fatalf("seed permit: %v", err)
	}
	if _, err := admission.ReleaseOrdinary(requested.Ref); err != nil {
		t.Fatalf("seed ordinary release: %v", err)
	}
	seeded = append(seeded, permit)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := kernel.SubmitAndWait(ctx, Request{
		UID: "boundary-replacement", LaneKey: "secret-store:replacement", Source: lifecycle.SourceJobManager, Route: "replacement",
	}); err != nil {
		t.Fatalf("maximum-population replacement failed: %v", err)
	}
	if replacementPrepared.Load() != 1 {
		t.Fatalf("replacement prepare calls=%d want=1", replacementPrepared.Load())
	}
	err = kernel.SubmitAndWait(ctx, Request{
		UID: "boundary-addition", LaneKey: "secret-store:addition", Source: lifecycle.SourceJobManager, Route: "addition",
	})
	if !errors.Is(err, lifecycle.ErrLongLivedRecordCapacity) {
		t.Fatalf("steady addition capacity result differs: %v", err)
	}
	if additionPrepared.Load() != 0 {
		t.Fatalf("capacity-rejected addition reached Prepare: calls=%d", additionPrepared.Load())
	}
	if !run.Admitting() || run.DirtyCause() != nil {
		t.Fatalf("steady capacity rejection poisoned Kernel: admitting=%v dirty=%v", run.Admitting(), run.DirtyCause())
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.LongLivedRecords != 1 ||
		census.OrdinaryGranted != 0 {
		t.Fatalf("boundary operations left Admission ownership: %+v", census)
	}
	if census := tasks.LongLivedCensus(); census.Active != 1 ||
		census.SecretStores != 1 ||
		census.Bytes != steadyPlan.Bytes() {
		t.Fatalf("boundary operations left long-lived ownership: %+v", census)
	}
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if census := tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("boundary finalizer retained permits: %+v", census)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelRunFinalizerFailureReleasesTaskWithoutQuiescence(t *testing.T) {
	want := errors.New("finalizer failed")
	finalizer := RunFinalizerFunc(func(context.Context, uint64) error { return want })
	kernel, run, _, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); !errors.Is(err, want) {
		t.Fatalf("finalizer failure differs: %v", err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf("failed finalizer retained transient task: active=%d pending=%d", tasks.Active(), tasks.Pending())
	}
	terminal := run.TerminalState()
	if !terminal.Reached || terminal.Quiescent || !errors.Is(terminal.Dirty, want) {
		t.Fatalf("failed finalizer terminal differs: %+v", terminal)
	}
}

func TestKernelRunFinalizerPanicReleasesTaskWithoutQuiescence(t *testing.T) {
	finalizer := RunFinalizerFunc(func(context.Context, uint64) error { panic("finalizer panic") })
	kernel, run, _, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); !errors.Is(err, lifecycle.ErrTaskPanic) {
		t.Fatalf("finalizer panic differs: %v", err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf("panicked finalizer retained transient task: active=%d pending=%d", tasks.Active(), tasks.Pending())
	}
	terminal := run.TerminalState()
	if !terminal.Reached || terminal.Quiescent || !errors.Is(terminal.Dirty, lifecycle.ErrTaskPanic) {
		t.Fatalf("panicked finalizer terminal differs: %+v", terminal)
	}
}

func TestKernelRunFinalizerRejectsUnrelatedLongLivedPermit(t *testing.T) {
	var calls atomic.Int32
	finalizer := RunFinalizerFunc(func(context.Context, uint64) error {
		calls.Add(1)
		return nil
	})
	kernel, run, admission, _, tasks := newKernelWithPlannerWriterFinalizerAndTimeout(t, stoppedKernelPlanner{}, io.Discard, finalizer, 10*time.Millisecond)
	plan, err := lifecycle.NewJobLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
	}
	requested := admission.RequestOrdinary(
		run.Generation(),
		lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1},
		plan.Bytes()+512,
	)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	permit, err := tasks.IssueLongLivedPermit(admission, requested.Ref, lifecycle.ResourceIdentity{ID: "job", Generation: 1}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(requested.Ref); err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), "shutdown deadline exceeded") {
		t.Fatalf("unrelated long-lived terminal differs: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("finalizer ran with an unrelated long-lived permit: calls=%d", calls.Load())
	}
	if err := permit.AbortUnused(); err != nil {
		t.Fatal(err)
	}
}

func TestKernelRejectsMissingRunFinalizer(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	planner := stoppedKernelPlanner{}
	if _, err := NewCommandKernel(
		run, admission, uids, tasks, frames, lifecycle.RealClock{},
		make(chan lifecycle.AdmissionGrant, 1), nil,
		newNoopRunShutdownBarrier(), nil,
		planner, newTestFunctionCatalog(planner),
	); err == nil {
		t.Fatal("Kernel accepted missing run finalizer")
	}
}

func TestKernelCannotReportQuiescentWithRetainedLongLivedPermit(t *testing.T) {
	kernel, run, admission, _, tasks := newKernelWithPlannerAndTimeout(t, stoppedKernelPlanner{}, time.Millisecond)
	permitPlan, err := lifecycle.NewJobLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
	}
	requested := admission.RequestOrdinary(
		run.Generation(),
		lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1},
		permitPlan.Bytes()+512,
	)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant count=%d grant=%+v err=%v", count, grants[0], err)
	}
	permit, err := tasks.IssueLongLivedPermit(
		admission, requested.Ref, lifecycle.ResourceIdentity{ID: "retained", Generation: 1}, permitPlan,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err == nil || !strings.Contains(err.Error(), "shutdown deadline exceeded") || !strings.Contains(err.Error(), "nonzero process census") {
		t.Fatalf("retained permit terminal error=%v", err)
	}
	if census := tasks.LongLivedCensus(); census.Active != 1 ||
		census.Bytes != permitPlan.Bytes() ||
		census.ExternalReserved != 1 {
		t.Fatalf("retained permit census=%+v", census)
	}
	if err := permit.AbortUnused(); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(requested.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestKernelStopDrainsCooperativeTask(t *testing.T) {
	started := make(chan struct{})
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			CooperativeCancel: true,
			Work: lifecycle.FrameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
				close(started)
				<-ctx.Done()
				return lifecycle.NewSealedResult(499, "application/json", []byte(`{"status":499}`))
			}),
		}, nil
	})
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string { return "lane" })
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{
		UID: "cooperative-stop", Source: lifecycle.SourceFunction,
		Route: "route", Deadline: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cooperative task did not start")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatalf("cooperative shutdown did not drain: %v", err)
	}
	if tasks.Active() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("cooperative shutdown retained task/kernel state: tasks=%d operations=%d lanes=%d", tasks.Active(), len(kernel.operations), len(kernel.lanes))
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.Phase != "cleanup-only" {
		t.Fatalf("cooperative shutdown admission census differs: %#v", census)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelShutdownSettlesPendingInputBodyGrowthBeforeCleanupOnly(t *testing.T) {
	run, err := lifecycle.NewRunSupervisor(1, lifecycle.RealClock{}, lifecycle.DefaultShutdownTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.FinishShutdown() })
	uids := lifecycle.NewUIDLedger()
	admission := lifecycle.NewAdmissionLedger()
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	grants := make(chan lifecycle.AdmissionGrant, 1)
	planner := stoppedKernelPlanner{}
	kernel, err := NewCommandKernel(
		run, admission, uids, tasks, frames, lifecycle.RealClock{}, grants, nil,
		newNoopRunShutdownBarrier(), newNoopRunFinalizer(),
		planner, newTestFunctionCatalog(planner),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	const capacity = int64(64 * 1024)
	token, err := admission.RequestInputBodyGrowth(run.Generation(), 0, capacity)
	if err != nil {
		t.Fatal(err)
	}
	if err := kernel.beginShutdown(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	select {
	case grant := <-grants:
		if grant.Kind != lifecycle.ReservationInputBodyGrowth || grant.InputBodyToken != token || grant.Bytes != capacity {
			t.Fatalf("shutdown input grant differs: %+v", grant)
		}
	default:
		t.Fatal("shutdown did not settle the pending input body growth")
	}
	if census := admission.Census(); census.Phase != "cleanup-only" || census.InputBodyWaiting {
		t.Fatalf("shutdown admission transition differs: %+v", census)
	}
	if _, err := admission.CommitInputBodyGrowth(token, capacity); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.AbortInputBody(token); err != nil {
		t.Fatal(err)
	}
	if !kernel.shutdownQuiescent() {
		t.Fatal("shutdown did not become quiescent after parser released its body")
	}
}

func TestKernelShutdownCancelsOperationsInBoundedTurns(t *testing.T) {
	const (
		population = 257
		quantum    = 4
	)
	kernel, run := newKernel(t)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	plan := WorkPlan{
		Work:       lifecycle.FrameTaskWork(plannerPlanWork),
		NoResponse: true,
	}
	for index := range population {
		request := Request{
			UID:     fmt.Sprintf("shutdown-operation-%d", index),
			Source:  lifecycle.SourceJobManager,
			LaneKey: "shared",
		}
		if err := kernel.admit(request, plan, nil, nil, nil); err != nil {
			t.Fatalf("admit operation %d: %v", index, err)
		}
	}
	if err := kernel.beginShutdown(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		before := len(kernel.operations)
		more, err := kernel.serviceShutdownOperations(quantum)
		if err != nil {
			t.Fatal(err)
		}
		visited := before - len(kernel.operations)
		if visited < 0 || visited > quantum {
			t.Fatalf(
				"one shutdown turn disposed %d operations, want 0..%d",
				visited,
				quantum,
			)
		}
		if !more {
			break
		}
	}
	if len(kernel.operations) != 0 || kernel.operationHead != nil ||
		kernel.operationTail != nil {
		t.Fatalf(
			"shutdown retained operations=%d head=%p tail=%p",
			len(kernel.operations),
			kernel.operationHead,
			kernel.operationTail,
		)
	}
	if err := kernel.advanceShutdownAdmission(); err != nil {
		t.Fatal(err)
	}
	if _, err := kernel.serviceShutdownStops(quantum); err != nil {
		t.Fatal(err)
	}
	if len(kernel.lanes) != 0 || kernel.laneHead != nil ||
		kernel.laneTail != nil {
		t.Fatalf(
			"shutdown retained lanes=%d head=%p tail=%p",
			len(kernel.lanes),
			kernel.laneHead,
			kernel.laneTail,
		)
	}
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
			if err := run.OpenAdmission(); err != nil {
				t.Fatal(err)
			}
			for index := range population {
				key := fmt.Sprintf("shutdown-lane-%d", index)
				if _, err := kernel.allocateLane(
					commandLaneKey{
						source: lifecycle.SourceJobManager,
						key:    key,
					},
					Request{
						Source:  lifecycle.SourceJobManager,
						LaneKey: key,
					},
				); err != nil {
					t.Fatal(err)
				}
			}
			if err := kernel.beginShutdown(
				time.Now().Add(time.Second),
			); err != nil {
				t.Fatal(err)
			}
			for {
				before := len(kernel.lanes)
				more, err := kernel.serviceShutdownStops(quantum)
				if err != nil {
					t.Fatal(err)
				}
				visited := before - len(kernel.lanes)
				if visited < 0 || visited > quantum {
					t.Fatalf(
						"one shutdown turn visited %d lanes, want 0..%d",
						visited,
						quantum,
					)
				}
				if !more {
					break
				}
			}
			if len(kernel.lanes) != 0 || kernel.laneHead != nil ||
				kernel.laneTail != nil {
				t.Fatalf(
					"shutdown retained lanes=%d head=%p tail=%p",
					len(kernel.lanes),
					kernel.laneHead,
					kernel.laneTail,
				)
			}
		})
	}
}

func TestKernelRunsTaskCleanupBeforeSlotRelease(t *testing.T) {
	cleaned := make(chan struct{}, 2)
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
				return lifecycle.NewSealedResult(200, "application/json", []byte(`{"status":200}`))
			}),
			Cleanup: func() error {
				cleaned <- struct{}{}
				return nil
			},
		}, nil
	})
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, planner)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{
		UID: "cleanup", LaneKey: "lane", Source: lifecycle.SourceJobManager,
		Route: "route", Deadline: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cleaned:
	case <-time.After(time.Second):
		t.Fatal("task cleanup phase did not execute")
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatalf("cleanup task did not drain: %v", err)
	}
	select {
	case <-cleaned:
		t.Fatal("task cleanup phase executed more than once")
	default:
	}
	if tasks.Active() != 0 || len(kernel.operations) != 0 || len(kernel.lanes) != 0 {
		t.Fatalf("cleanup task retained state: tasks=%d operations=%d lanes=%d", tasks.Active(), len(kernel.operations), len(kernel.lanes))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelCancelsQueuedOperationWithoutStartingWork(t *testing.T) {
	started := make(chan struct{})
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		switch route {
		case "blocking":
			return WorkPlan{
				CooperativeCancel: true,
				Work: lifecycle.FrameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					close(started)
					<-ctx.Done()
					return lifecycle.NewControlResult(lifecycle.ControlCancelled)
				}),
			}, nil
		case "queued":
			return WorkPlan{
				Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
					t.Fatal("cancelled queued operation entered TaskChild")
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				}),
			}, nil
		default:
			return WorkPlan{}, errors.New("unexpected route")
		}
	})
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(FunctionLookup) string { return "lane" })
	catalog := testFunctionCatalogFor(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	deadline := time.Now().Add(time.Minute)
	if err := kernel.Submit(context.Background(), Request{UID: "blocking", Source: lifecycle.SourceFunction, Route: "blocking", Deadline: deadline}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking operation did not start")
	}
	queuedResult := make(chan error, 1)
	if err := kernel.submit(context.Background(), Request{
		UID: "queued", Source: lifecycle.SourceFunction, Route: "queued", Deadline: deadline,
	}, queuedResult); err != nil {
		t.Fatal(err)
	}
	if err := kernel.Cancel(context.Background(), "queued"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-queuedResult:
		if err != nil {
			t.Fatalf("queued cancellation result differs: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued cancellation did not reach terminal disposal")
	}
	if catalog.next != 2 || catalog.release != 1 || len(catalog.leases) != 1 {
		t.Fatalf("queued cancellation Function lease differs: resolves=%d releases=%d active=%d",
			catalog.next, catalog.release, len(catalog.leases))
	}

	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if tasks.Active() != 0 || tasks.Pending() != 0 {
		t.Fatalf("task census differs: active=%d pending=%d", tasks.Active(), tasks.Pending())
	}
	if catalog.next != 2 || catalog.release != 2 || len(catalog.leases) != 0 {
		t.Fatalf("shutdown Function lease differs: resolves=%d releases=%d active=%d",
			catalog.next, catalog.release, len(catalog.leases))
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelReservesExactPlanAndFrameBytesBeforeWrite(t *testing.T) {
	pad, err := lifecycle.RepeatedStringValue(1024*1024, 'A')
	if err != nil {
		t.Fatal(err)
	}
	body, err := lifecycle.ObjectValue(lifecycle.ObjectField{Key: "pad", Value: pad})
	if err != nil {
		t.Fatal(err)
	}
	result, err := lifecycle.NewCompleteRawValueEnvelope(200, lifecycle.ReviewedPerformanceJSON, body)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := lifecycle.SealFunctionResult(result)
	if err != nil {
		t.Fatal(err)
	}
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) { return sealed, nil })}, nil
	})
	writer := &holdingFrameWriter{offered: make(chan []byte, 1), release: make(chan struct{})}
	kernel, run, admission, uids, _ := newKernelWithPlannerAndWriter(t, planner, writer)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	request := Request{UID: "self-fit", Source: lifecycle.SourceFunction, Route: "route"}
	if err := kernel.Submit(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var frame []byte
	select {
	case frame = <-writer.offered:
	case <-time.After(time.Second):
		t.Fatal("result did not reach held Write")
	}
	chargedRequest := request
	chargedRequest.LaneKey = request.Route
	base, err := operationAdmissionBytes(chargedRequest, WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)})
	if err != nil {
		t.Fatal(err)
	}
	const repeatedObjectPlanBytes = int64(64 + 16 + len("pad") + 64)
	wantBytes := base + repeatedObjectPlanBytes + int64(len(frame))
	if census := admission.Census(); census.OrdinaryBytes != wantBytes || census.OrdinaryGranted != 1 || census.OrdinaryWaiting != 0 {
		t.Fatalf("held-write admission differs: got=%#v want-bytes=%d", census, wantBytes)
	}
	close(writer.release)
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestOperationAdmissionBytesIncludesSealedRequestMetadata(t *testing.T) {
	baseRequest := Request{
		UID: "metadata", Source: lifecycle.SourceFunction, Route: "route",
	}
	plan := WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)}
	base, err := operationAdmissionBytes(baseRequest, plan)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		mutate func(*Request)
		delta  int64
	}{
		"content type": {
			mutate: func(request *Request) { request.ContentType = "application/json" },
			delta:  int64(len("application/json")),
		},
		"argument storage": {
			mutate: func(request *Request) { request.Args = []string{"a", "bc"} },
			delta:  int64(len("a") + len("bc") + 2*16),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := baseRequest
			test.mutate(&request)
			got, err := operationAdmissionBytes(request, plan)
			if err != nil {
				t.Fatal(err)
			}
			if got != base+test.delta {
				t.Fatalf("admission metadata delta differs: got=%d want=%d", got-base, test.delta)
			}
		})
	}
}

func TestOperationAdmissionBytesIncludesTaskChildExecutionAllowance(t *testing.T) {
	got, err := operationAdmissionBytes(Request{}, WorkPlan{})
	if err != nil {
		t.Fatal(err)
	}
	want := operationRecordAdmissionBytes + lifecycle.TaskChildExecutionBytes
	if got != want {
		t.Fatalf("empty operation admission=%d, want %d", got, want)
	}
}

func TestOperationResultAdmissionBytesBoundaries(t *testing.T) {
	base := int64(512)
	tests := map[string]struct {
		frame int64
		valid bool
	}{
		"below deferred boundary": {frame: lifecycle.OrdinaryBudgetBytes - base - 2, valid: true},
		"at deferred boundary":    {frame: lifecycle.OrdinaryBudgetBytes - base - 1, valid: true},
		"over deferred boundary":  {frame: lifecycle.OrdinaryBudgetBytes - base, valid: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			total, err := operationResultAdmissionBytes(base, lifecycle.ResultPreflight{PlanBytes: 1, FrameBytes: test.frame})
			if (err == nil) != test.valid {
				t.Fatalf("result self-fit differs: total=%d err=%v", total, err)
			}
		})
	}
}

func TestKernelCancelsResultWaitingForAdmissionGrowth(t *testing.T) {
	pad, err := lifecycle.RepeatedStringValue(1024*1024, 'A')
	if err != nil {
		t.Fatal(err)
	}
	body, err := lifecycle.ObjectValue(lifecycle.ObjectField{Key: "pad", Value: pad})
	if err != nil {
		t.Fatal(err)
	}
	result, err := lifecycle.NewCompleteRawValueEnvelope(200, lifecycle.ReviewedPerformanceJSON, body)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := lifecycle.SealFunctionResult(result)
	if err != nil {
		t.Fatal(err)
	}
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) { return sealed, nil })}, nil
	})
	kernel, run, admission, uids, _ := newKernelWithPlanner(t, planner)
	blocker := admission.RequestOrdinary(run.Generation(), lifecycle.AdmissionLaneRef{Slot: ^uint32(0), Generation: 1}, lifecycle.OrdinaryBudgetBytes-1024*1024)
	if blocker.Rejected != nil {
		t.Fatal(blocker.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	if count, _, err := admission.TakeGrants(1, &grants); err != nil || count != 1 || grants[0].Ref != blocker.Ref {
		t.Fatalf("blocker grant differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{UID: "growth-cancel", Source: lifecycle.SourceFunction, Route: "route"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		census := admission.Census()
		if census.OrdinaryWaiting == 1 && census.OrdinaryGranted == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if census := admission.Census(); census.OrdinaryWaiting != 1 || census.OrdinaryGranted != 2 {
		t.Fatalf("result growth did not wait: %#v", census)
	}
	if err := kernel.Cancel(context.Background(), "growth-cancel"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for admission.Census().OrdinaryWaiting != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if census := admission.Census(); census.OrdinaryWaiting != 0 || census.OrdinaryGranted < 1 || census.OrdinaryGranted > 2 {
		t.Fatalf("cancelled growth retained waiter: %#v", census)
	}
	if _, err := admission.ReleaseOrdinary(blocker.Ref); err != nil {
		t.Fatal(err)
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func plannerPlanWork(context.Context) (lifecycle.SealedResult, error) {
	return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
}

type holdingFrameWriter struct {
	offered chan []byte
	release chan struct{}
}

func (writer *holdingFrameWriter) Write(payload []byte) (int, error) {
	writer.offered <- bytes.Clone(payload)
	<-writer.release
	return len(payload), nil
}

type firstHoldingFrameWriter struct {
	once    sync.Once
	offered chan []byte
	release chan struct{}
}

func (writer *firstHoldingFrameWriter) Write(payload []byte) (int, error) {
	writer.once.Do(func() {
		writer.offered <- bytes.Clone(payload)
		<-writer.release
	})
	return len(payload), nil
}

func TestKernelExternalSubmissionServiceRotatesSources(t *testing.T) {
	kernel, run := newKernel(t)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	requests := []Request{
		{UID: "j1", LaneKey: "j1", Source: lifecycle.SourceJobManager, Route: "route"},
		{UID: "j2", LaneKey: "j2", Source: lifecycle.SourceJobManager, Route: "route"},
		{UID: "f1", Source: lifecycle.SourceFunction, Route: "route"},
		{UID: "f2", Source: lifecycle.SourceFunction, Route: "route"},
	}
	results := make([]chan error, len(requests))
	for index, request := range requests {
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		if err != nil {
			t.Fatal(err)
		}
		results[index] = make(chan error, 1)
		kernel.submissions[sourceIndex(request.Source)] <- submission{request: request, plan: plan, result: results[index]}
	}
	kernel.serviceSubmissions(4)
	kernel.serviceAdmissions(4)
	for _, result := range results {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]lifecycle.OperationID{"j1": 1, "f1": 2, "j2": 3, "f2": 4}
	for uid, id := range want {
		if operation := kernel.operations[uid]; operation == nil || operation.ID != id {
			t.Fatalf("external source rotation differs for %s: %#v", uid, operation)
		}
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
		if err != nil {
			t.Fatal(err)
		}
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
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	for index, result := range results {
		select {
		case err := <-result:
			if err == nil || !strings.Contains(err.Error(), "admission closed") {
				t.Fatalf("submission %d result differs: %v", index, err)
			}
		default:
			t.Fatalf("submission %d was not drained", index)
		}
	}
}

func TestKernelClosedAdmissionDoesNotRearmFrameBlockedControl(t *testing.T) {
	kernel, _ := newKernel(t)
	source := sourceIndex(lifecycle.SourceFunction)
	kernel.blockedSubmission[source] = true
	kernel.blockedSubmissions[source] = submission{controlStatus: lifecycle.ControlBadRequest}
	if kernel.hasRunnableSubmissions() {
		t.Fatal("frame-blocked control was reported as immediately runnable")
	}
}

func TestKernelTaskSchedulingCountsClaimConflictsAgainstQuantum(t *testing.T) {
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{
			Claims: []string{"shared"},
			Work:   lifecycle.FrameTaskWork(plannerPlanWork),
		}, nil
	})
	kernel, run, _, _, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(lookup FunctionLookup) string { return lookup.UID })
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 9; index++ {
		request := Request{
			UID:    fmt.Sprintf("claim-%d", index),
			Source: lifecycle.SourceFunction,
			Route:  "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := kernel.admit(request, plan, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		kernel.serviceAdmissions(4)
	}
	if got := kernel.ready[0].len + kernel.ready[1].len; got != 9 {
		t.Fatalf("initial ready lanes differ: %d", got)
	}
	if more := kernel.scheduleTasks(4); !more || tasks.Pending() != 1 || kernel.claims.WaitingCount() != 3 || kernel.ready[0].len+kernel.ready[1].len != 5 {
		t.Fatalf("first quantum differs: more=%v pending=%d waiters=%d ready=%d", more, tasks.Pending(), kernel.claims.WaitingCount(), kernel.ready[0].len+kernel.ready[1].len)
	}
	if more := kernel.scheduleTasks(4); !more || tasks.Pending() != 1 || kernel.claims.WaitingCount() != 7 || kernel.ready[0].len+kernel.ready[1].len != 1 {
		t.Fatalf("second quantum differs: more=%v pending=%d waiters=%d ready=%d", more, tasks.Pending(), kernel.claims.WaitingCount(), kernel.ready[0].len+kernel.ready[1].len)
	}
	if more := kernel.scheduleTasks(4); more || tasks.Pending() != 1 || kernel.claims.WaitingCount() != 8 || kernel.ready[0].len+kernel.ready[1].len != 0 {
		t.Fatalf("final quantum differs: more=%v pending=%d waiters=%d ready=%d", more, tasks.Pending(), kernel.claims.WaitingCount(), kernel.ready[0].len+kernel.ready[1].len)
	}
}

func TestKernelResourceScopedFunctionHasIndependentTaskSchedulingClass(t *testing.T) {
	planner := plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
		return WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)}, nil
	})
	kernel, run, _, _, tasks := newKernelWithPlanner(t, planner)
	setTestFunctionResource(t, kernel, func(lookup FunctionLookup) string {
		if lookup.Route == "dyncfg" {
			return "dyncfg-resource"
		}
		return ""
	})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	requests := []Request{
		{UID: "generic-first", Source: lifecycle.SourceFunction, Route: "generic"},
		{UID: "dyncfg-second", Source: lifecycle.SourceFunction, Route: "dyncfg"},
	}
	for _, request := range requests {
		if err := kernel.admit(request, WorkPlan{}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	kernel.serviceAdmissions(len(requests))
	if more := kernel.scheduleTasks(len(requests)); more {
		t.Fatal("task scheduling retained an unexpected ready lane")
	}
	if tasks.Pending() != len(requests) {
		t.Fatalf("pending tasks=%d, want %d", tasks.Pending(), len(requests))
	}
	var started [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, _, err := tasks.Dispatch(context.Background(), 1, &started)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("started tasks=%d, want 1", count)
	}
	operation := kernel.tasksByRequest[started[0].Request]
	if operation == nil {
		t.Fatal("started task has no kernel operation")
	}
	if operation.Source != lifecycle.SourceFunction {
		t.Fatalf("resource-scoped Function source=%v, want Function", operation.Source)
	}
	if operation.request.UID != "dyncfg-second" {
		t.Fatalf(
			"first dispatched Function=%q, want resource-scoped %q",
			operation.request.UID,
			"dyncfg-second",
		)
	}
}

func TestKernelSubmissionBacklogCannotStarveStop(t *testing.T) {
	kernel, run, admission, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < externalSourceQueueDepth; index++ {
		request := Request{
			UID:    fmt.Sprintf("backlog-%d", index),
			Source: lifecycle.SourceFunction, Route: "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		if err != nil {
			t.Fatal(err)
		}
		kernel.submissions[sourceIndex(request.Source)] <- submission{
			request: request, plan: plan, result: make(chan error, 1),
		}
	}
	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.WaitShutdownStarted(ctx); err != nil {
		t.Fatal(err)
	}
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelSubmissionBacklogCannotStarveDueDeadline(t *testing.T) {
	var output bytes.Buffer
	kernel, run, admission, uids, _ := newKernelWithPlannerAndWriter(t, stoppedKernelPlanner{}, &output)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	request := Request{
		UID: "deadline-probe", Source: lifecycle.SourceFunction, Route: "route",
		Deadline: time.Now().Add(-time.Second),
	}
	plan, err := kernel.prepareSubmissionPlanForTest(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := kernel.admit(request, plan, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < externalSourceQueueDepth; index++ {
		request := Request{
			UID:    fmt.Sprintf("backlog-%d", index),
			Source: lifecycle.SourceFunction, Route: "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		if err != nil {
			t.Fatal(err)
		}
		kernel.submissions[sourceIndex(request.Source)] <- submission{
			request: request, plan: plan, result: make(chan error, 1),
		}
	}
	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN deadline-probe 504 application/json ")) {
		t.Fatalf("due deadline was starved or overwritten by shutdown: %q", output.Bytes())
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelPreAdmissionRejectionCommitsWithoutUIDOrAdmissionRecord(t *testing.T) {
	var output bytes.Buffer
	kernel, run, admission, uids, _ := newKernelWithPlannerAndWriter(t, stoppedKernelPlanner{}, &output)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Reject(context.Background(), "malformed-safe-uid", lifecycle.ControlBadRequest); err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.OrdinaryWaiting != 0 || census.OrdinaryGranted != 0 {
		t.Fatalf("pre-admission rejection consumed admission state: %#v", census)
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN malformed-safe-uid 400 application/json ")) {
		t.Fatalf("pre-admission rejection frame differs: %q", output.Bytes())
	}
	kernel.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admission.CloseDrained(run.Generation()); err != nil {
		t.Fatal(err)
	}
	closeUIDLedger(t, uids)
}

func TestKernelDeadlineServiceHasFixedQuantum(t *testing.T) {
	kernel, _ := newKernel(t)
	now := time.Now()
	for index := 0; index < 9; index++ {
		id := lifecycle.OperationID(index + 1)
		generation, err := lifecycle.NewOperation(id, fmt.Sprintf("u%d", index), lifecycle.SourceFunction, fmt.Sprintf("lane%d", index), true)
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range []lifecycle.OperationState{
			lifecycle.OperationQueued, lifecycle.OperationAcquiringClaims, lifecycle.OperationReady, lifecycle.OperationRunning,
		} {
			if err := generation.Advance(state); err != nil {
				t.Fatal(err)
			}
		}
		if err := generation.StartChild(lifecycle.TaskRef{Slot: uint32(index), Generation: uint64(index + 1)}); err != nil {
			t.Fatal(err)
		}
		operation := &commandOperation{OperationGeneration: generation, deadline: deadlineEntry{when: now.Add(-time.Second), index: -1}}
		operation.deadline.operation = operation
		heap.Push(&kernel.deadlines, &operation.deadline)
	}
	if more := kernel.serviceDeadlines(now, 4); !more || len(kernel.controls) != 4 || kernel.deadlines.Len() != 5 {
		t.Fatalf("first deadline quantum differs: more=%v controls=%d deadlines=%d", more, len(kernel.controls), kernel.deadlines.Len())
	}
	if more := kernel.serviceDeadlines(now, 4); !more || len(kernel.controls) != 8 || kernel.deadlines.Len() != 1 {
		t.Fatalf("second deadline quantum differs: more=%v controls=%d deadlines=%d", more, len(kernel.controls), kernel.deadlines.Len())
	}
	if more := kernel.serviceDeadlines(now, 4); more || len(kernel.controls) != 9 || kernel.deadlines.Len() != 0 {
		t.Fatalf("final deadline quantum differs: more=%v controls=%d deadlines=%d", more, len(kernel.controls), kernel.deadlines.Len())
	}
}

func newStoppedKernel(t *testing.T) *CommandKernel {
	t.Helper()
	kernel, _ := newKernel(t)
	startKernelLoop(t, kernel)
	kernel.Stop()
	if err := kernel.Wait(context.Background()); err != nil {
		t.Fatalf("first wait differs: %v", err)
	}
	return kernel
}

func newKernel(t *testing.T) (*CommandKernel, *lifecycle.RunSupervisor) {
	t.Helper()
	kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	return kernel, run
}

func newKernelWithPlanner(t *testing.T, planner Planner) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerAndWriter(t, planner, io.Discard)
}

func newKernelWithPlannerAndTimeout(t *testing.T, planner Planner, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerWriterAndTimeout(t, planner, io.Discard, timeout)
}

func newKernelWithPlannerAndWriter(t *testing.T, planner Planner, writer io.Writer) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerWriterAndTimeout(t, planner, writer, lifecycle.DefaultShutdownTimeout)
}

func newKernelWithPlannerWriterAndTimeout(t *testing.T, planner Planner, writer io.Writer, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithPlannerWriterFinalizerAndTimeout(t, planner, writer, newNoopRunFinalizer(), timeout)
}

func newKernelWithPlannerWriterFinalizerAndTimeout(t *testing.T, planner Planner, writer io.Writer, finalizer RunFinalizer, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithClockFinalizerAndTimeout(t, planner, writer, lifecycle.RealClock{}, finalizer, timeout)
}

func newKernelWithClockFinalizerAndTimeout(t *testing.T, planner Planner, writer io.Writer, clock lifecycle.Clock, finalizer RunFinalizer, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithClockFinalizerCatalogAndTimeout(
		t, planner, newTestFunctionCatalog(planner), writer, clock, finalizer, timeout,
	)
}

func newKernelWithClockFinalizerCatalogAndTimeout(t *testing.T, planner Planner, functionCatalog FunctionCatalogPort, writer io.Writer, clock lifecycle.Clock, finalizer RunFinalizer, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	t.Helper()
	run, err := lifecycle.NewRunSupervisor(1, clock, timeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.FinishShutdown() })
	uids := lifecycle.NewUIDLedger()
	admission := lifecycle.NewAdmissionLedger()
	frames, err := lifecycle.NewFrameOwner(writer)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := NewCommandKernel(
		run, admission, uids, tasks, frames, clock,
		make(chan lifecycle.AdmissionGrant, 1), nil,
		newNoopRunShutdownBarrier(), finalizer,
		planner, functionCatalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	return kernel, run, admission, uids, tasks
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

func (clock *kernelFinalizerClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *kernelFinalizerClock) Arm(kind string, delay time.Duration) (<-chan time.Time, func()) {
	ready := make(chan time.Time, 1)
	clock.mu.Lock()
	if kind == lifecycle.TimerKindShutdown {
		clock.shutdown = ready
	} else if kind == lifecycle.TimerKindDeadline {
		clock.deadlineArms++
		select {
		case clock.deadlineArmed <- struct{}{}:
		default:
		}
	}
	clock.mu.Unlock()
	return ready, func() {}
}

func (clock *kernelFinalizerClock) deadlineArmCount() int {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.deadlineArms
}

func (clock *kernelFinalizerClock) expireShutdown(t *testing.T) {
	t.Helper()
	clock.mu.Lock()
	ready := clock.shutdown
	clock.now = clock.now.Add(time.Second)
	now := clock.now
	clock.mu.Unlock()
	if ready == nil {
		t.Fatal("shutdown timer was not armed")
	}
	ready <- now
}

func (clock *kernelFinalizerClock) advanceShutdownWithoutSignal(t *testing.T) {
	t.Helper()
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if clock.shutdown == nil {
		t.Fatal("shutdown timer was not armed")
	}
	clock.now = clock.now.Add(time.Second)
}

func (clock *kernelFinalizerClock) advance(delay time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(delay)
	clock.mu.Unlock()
}

func closeUIDLedger(t *testing.T, ledger *lifecycle.UIDLedger) {
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

type plannerFunc func(context.Context, string, []string) (WorkPlan, error)

func (fn plannerFunc) Plan(request Request) (WorkPlan, error) {
	return fn(context.Background(), request.Route, request.Args)
}

func (kernel *CommandKernel) prepareSubmissionPlanForTest(request Request) (WorkPlan, error) {
	if request.Source == lifecycle.SourceFunction {
		return WorkPlan{}, nil
	}
	return kernel.prepareJobPlan(request)
}

type testFunctionCatalog struct {
	planner  Planner
	resource func(FunctionLookup) string
	next     uint64
	release  uint64
	peak     int
	leases   map[FunctionInvocationRef]struct{}
}

type functionCatalogPortStub struct {
	resolve  func(FunctionLookup) (FunctionCatalogDecision, error)
	release  func(FunctionInvocationRef) (FunctionCleanupPlan, error)
	complete func(FunctionCleanupRef, error) error
}

func (catalog functionCatalogPortStub) ResolveAndAcquire(lookup FunctionLookup) (FunctionCatalogDecision, error) {
	return catalog.resolve(lookup)
}

func (catalog functionCatalogPortStub) ReleaseInvocation(ref FunctionInvocationRef) (FunctionCleanupPlan, error) {
	return catalog.release(ref)
}

func (catalog functionCatalogPortStub) CompleteCleanup(ref FunctionCleanupRef, err error) error {
	if catalog.complete == nil {
		return nil
	}
	return catalog.complete(ref, err)
}

func (functionCatalogPortStub) BeginMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: mutations unsupported")
}

func (functionCatalogPortStub) AdvanceMutationQuiesce(int) (FunctionCatalogMutationProgress, error) {
	return FunctionCatalogMutationProgress{}, errors.New(
		"test Function catalog: no active mutation",
	)
}

func (functionCatalogPortStub) ResumeMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) AdvanceMutation(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (FunctionCatalogMutationProgress, int, error) {
	return FunctionCatalogMutationProgress{}, 0, errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) AbortMutation(*[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, error) {
	return 0, errors.New("test Function catalog: no active mutation")
}

func (functionCatalogPortStub) BeginClose() error { return nil }

func (functionCatalogPortStub) CloseStep(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, bool, error) {
	return 0, false, nil
}

func (functionCatalogPortStub) LifecycleCensus() FunctionCatalogCensus {
	return FunctionCatalogCensus{Closed: true}
}

func newTestFunctionCatalog(planner Planner) *testFunctionCatalog {
	return &testFunctionCatalog{
		planner: planner,
		leases:  make(map[FunctionInvocationRef]struct{}),
	}
}

func (catalog *testFunctionCatalog) ResolveAndAcquire(lookup FunctionLookup) (FunctionCatalogDecision, error) {
	plan, err := catalog.planner.Plan(Request{
		UID: lookup.UID, Source: lifecycle.SourceFunction, Route: lookup.Route,
		Args: lookup.Args, Payload: lookup.Payload, ContentType: lookup.ContentType,
		Permissions: lookup.Permissions, CallerSource: lookup.CallerSource,
		Timeout: lookup.Timeout, HasPayload: lookup.HasPayload,
	})
	if err != nil {
		return FunctionCatalogDecision{}, err
	}
	catalog.next++
	ref := FunctionInvocationRef{Slot: 1, Generation: catalog.next}
	catalog.leases[ref] = struct{}{}
	if len(catalog.leases) > catalog.peak {
		catalog.peak = len(catalog.leases)
	}
	resourceID := ""
	if catalog.resource != nil {
		resourceID = catalog.resource(lookup)
	}
	return FunctionCatalogDecision{
		ResourceID: resourceID,
		Plan:       plan,
		Lease:      ref,
	}, nil
}

func (catalog *testFunctionCatalog) ReleaseInvocation(ref FunctionInvocationRef) (FunctionCleanupPlan, error) {
	if _, ok := catalog.leases[ref]; !ok {
		return FunctionCleanupPlan{}, errors.New("test Function catalog: unknown invocation lease")
	}
	delete(catalog.leases, ref)
	catalog.release++
	return FunctionCleanupPlan{}, nil
}

func (*testFunctionCatalog) CompleteCleanup(FunctionCleanupRef, error) error { return nil }

func (*testFunctionCatalog) BeginMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: mutations unsupported")
}

func (*testFunctionCatalog) AdvanceMutationQuiesce(int) (FunctionCatalogMutationProgress, error) {
	return FunctionCatalogMutationProgress{}, errors.New(
		"test Function catalog: no active mutation",
	)
}

func (*testFunctionCatalog) ResumeMutation(FunctionCatalogMutation) error {
	return errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) AdvanceMutation(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (FunctionCatalogMutationProgress, int, error) {
	return FunctionCatalogMutationProgress{}, 0, errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) AbortMutation(*[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, error) {
	return 0, errors.New("test Function catalog: no active mutation")
}

func (*testFunctionCatalog) BeginClose() error { return nil }

func (*testFunctionCatalog) CloseStep(int, *[MaximumFunctionCleanupBatch]FunctionCleanupPlan) (int, bool, error) {
	return 0, false, nil
}

func (*testFunctionCatalog) LifecycleCensus() FunctionCatalogCensus {
	return FunctionCatalogCensus{Closed: true}
}

func setTestFunctionResource(t *testing.T, kernel *CommandKernel, resource func(FunctionLookup) string) {
	t.Helper()
	testFunctionCatalogFor(t, kernel).resource = resource
}

func testFunctionCatalogFor(t *testing.T, kernel *CommandKernel) *testFunctionCatalog {
	t.Helper()
	catalog, ok := kernel.functionCatalog.(*testFunctionCatalog)
	if !ok {
		t.Fatal("kernel does not use the test Function catalog")
	}
	return catalog
}

func kernelResourcePlanner(t *testing.T, resource *kernelTestReadyResource, workEntered chan<- struct{}, workRelease <-chan struct{}) Planner {
	t.Helper()
	permitPlan, err := lifecycle.NewJobLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
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

func (planner kernelTestResourceSetPlanner) Plan(request Request) (WorkPlan, error) {
	if request.Route != "install" {
		return WorkPlan{}, errors.New("unexpected kernel resource-set route")
	}
	resource := planner.resources[request.LaneKey]
	if resource == nil {
		return WorkPlan{}, errors.New("unexpected kernel resource-set identity")
	}
	return WorkPlan{
		NoResponse: true,
		Resource: &ResourcePlan{
			Action: ResourceInstall,
			ID:     request.LaneKey,
			Permit: planner.permitPlan,
			Prepare: func(
				_ context.Context,
				generation uint64,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResource, error) {
				identity := lifecycle.ResourceIdentity{
					ID:         request.LaneKey,
					Generation: generation,
				}
				resource.identity = identity
				return &kernelTestPreparedResource{
					identity: identity,
					permit:   permit,
					ready:    resource,
				}, nil
			},
		},
	}, nil
}

type kernelTestTransactionPlanner struct {
	permitPlan          lifecycle.LongLivedPlan
	current             *kernelTestReadyResource
	successor           *kernelTestReadyResource
	prepareErr          error
	waitForCancellation bool
	events              *[]string
}

func (planner kernelTestTransactionPlanner) Plan(
	request Request,
) (WorkPlan, error) {
	switch request.Route {
	case "install":
		return WorkPlan{
			NoResponse: true,
			Resource: &ResourcePlan{
				Action: ResourceInstall,
				ID:     request.LaneKey,
				Permit: planner.permitPlan,
				Prepare: func(
					_ context.Context,
					generation uint64,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResource, error) {
					identity := lifecycle.ResourceIdentity{
						ID:         request.LaneKey,
						Generation: generation,
					}
					planner.current.identity = identity
					return &kernelTestPreparedResource{
						identity: identity,
						permit:   permit,
						ready:    planner.current,
					}, nil
				},
			},
		}, nil
	case "replace":
		return WorkPlan{
			Transaction: &ResourceTransactionPlan{
				ID:                request.LaneKey,
				AllocateSuccessor: true,
				Permit:            planner.permitPlan,
				Prepare: func(
					ctx context.Context,
					current lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					*planner.events = append(*planner.events, "prepare")
					if current != planner.current ||
						scope.Current != planner.current.identity ||
						!scope.Successor.Valid() {
						return nil, errors.New(
							"kernel test transaction scope differs",
						)
					}
					if planner.waitForCancellation {
						<-ctx.Done()
					}
					if planner.prepareErr != nil {
						return nil, planner.prepareErr
					}
					return &kernelTestPreparedResourceTransaction{
						scope:     scope,
						current:   planner.current,
						successor: planner.successor,
						permit:    permit,
						events:    planner.events,
					}, nil
				},
			},
		}, nil
	default:
		return WorkPlan{}, errors.New(
			"unexpected kernel transaction route",
		)
	}
}

type kernelTestPreparedResourceTransaction struct {
	scope     lifecycle.ResourceTransactionScope
	current   *kernelTestReadyResource
	successor *kernelTestReadyResource
	permit    lifecycle.LongLivedPermit
	events    *[]string
}

func (transaction *kernelTestPreparedResourceTransaction) Scope() lifecycle.ResourceTransactionScope {
	return transaction.scope
}

func (transaction *kernelTestPreparedResourceTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	*transaction.events = append(*transaction.events, "apply")
	if err := transaction.current.Stop(ctx); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if err := transaction.current.Finalize(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if err := transaction.permit.ActivateExternal(
		lifecycle.LongLivedEJobResources,
	); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	transaction.successor.identity = transaction.scope.Successor
	transaction.successor.permit = transaction.permit
	if err := transaction.successor.Publish(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	result, err := lifecycle.NewSealedResult(
		200,
		"application/json",
		[]byte(`{"accepted":true}`),
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		transaction.scope,
		lifecycle.ResourceTransactionReplaced,
		transaction.successor,
		result,
		func() error {
			*transaction.events = append(*transaction.events, "cleanup")
			return nil
		},
	)
}

func (transaction *kernelTestPreparedResourceTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	*transaction.events = append(*transaction.events, "dispose")
	return transaction.current, transaction.permit.AbortUnused()
}

func (planner kernelTestResourcePlanner) Plan(request Request) (WorkPlan, error) {
	switch request.Route {
	case "install":
		return WorkPlan{
			NoResponse: true,
			Resource: &ResourcePlan{
				Action: ResourceInstall,
				ID:     request.LaneKey,
				Permit: planner.permitPlan,
				Prepare: func(_ context.Context, generation uint64, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResource, error) {
					identity := lifecycle.ResourceIdentity{ID: request.LaneKey, Generation: generation}
					planner.resource.identity = identity
					return &kernelTestPreparedResource{
						identity: identity,
						permit:   permit,
						ready:    planner.resource,
					}, nil
				},
			},
		}, nil
	case "use":
		return WorkPlan{
			Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
				if planner.workEntered != nil {
					close(planner.workEntered)
				}
				if planner.workRelease != nil {
					<-planner.workRelease
				}
				return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
			}),
		}, nil
	default:
		return WorkPlan{}, errors.New("unexpected kernel resource route")
	}
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

type kernelTestPreparedResource struct {
	identity lifecycle.ResourceIdentity
	permit   lifecycle.LongLivedPermit
	ready    *kernelTestReadyResource
}

func (resource *kernelTestPreparedResource) Identity() lifecycle.ResourceIdentity {
	return resource.identity
}

func (resource *kernelTestPreparedResource) AcceptStart(_ context.Context, expected uint64) (lifecycle.ReadyResource, error) {
	if expected != resource.identity.Generation {
		return nil, errors.New("kernel test resource generation differs")
	}
	if err := resource.permit.ActivateExternal(lifecycle.LongLivedEJobResources); err != nil {
		return nil, err
	}
	resource.ready.permit = resource.permit
	return resource.ready, nil
}

func (resource *kernelTestPreparedResource) Dispose(context.Context) error {
	return resource.permit.AbortUnused()
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

func (resource *kernelTestReadyResource) Identity() lifecycle.ResourceIdentity {
	return resource.identity
}

func (resource *kernelTestReadyResource) Publish() error {
	resource.publishOnce.Do(func() { close(resource.publishEntered) })
	if resource.publishRelease != nil {
		<-resource.publishRelease
	}
	return nil
}

func (resource *kernelTestReadyResource) AbortReady(context.Context) error {
	return errors.Join(
		resource.permit.ReleaseExternal(lifecycle.LongLivedEJobResources),
		resource.permit.ReleaseBytes(),
		resource.permit.Return(),
	)
}

func (resource *kernelTestReadyResource) Stop(context.Context) error {
	resource.stopOnce.Do(func() { close(resource.stopEntered) })
	if resource.stopRelease != nil {
		<-resource.stopRelease
	}
	return resource.permit.ReleaseExternal(lifecycle.LongLivedEJobResources)
}

func (resource *kernelTestReadyResource) Finalize() error {
	return errors.Join(resource.permit.ReleaseBytes(), resource.permit.Return())
}

type stoppedKernelPlanner struct{}

func (stoppedKernelPlanner) Plan(Request) (WorkPlan, error) {
	return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})}, nil
}

func startKernelLoop(t *testing.T, kernel *CommandKernel) {
	t.Helper()
	loop, err := NewKernelLoop(kernel)
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}
