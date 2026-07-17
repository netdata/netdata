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
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := kernel.Submit(ctx, Request{UID: "after-stop", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"})
		if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "stopped") {
			t.Fatalf("post-stop submit differs: %v", err)
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
	}

	for name, test := range tests {
		t.Run(name, test.run)
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
	const helperEnv = "NETDATA_JOBMGRPOC_FINALIZER_DEADLINE_HELPER"
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
	const helperEnv = "NETDATA_JOBMGRPOC_FINALIZER_DUE_CLOCK_HELPER"
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
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, NoopRunFinalizer(), time.Second)
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
	kernel, run, admission, uids, _ := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	done := make(chan error, 1)
	go func() {
		done <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "one-deadline-timer", LaneKey: "timer", Source: lifecycle.SourceFunction, Route: "timer",
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
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, NoopRunFinalizer(), time.Second)
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
	blockerEntered := make(chan struct{}, lifecycle.TransientTaskSlots)
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
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	future := clock.Now().Add(time.Minute)
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		if err := kernel.Submit(context.Background(), Request{
			UID: fmt.Sprintf("blocker-%d", index), LaneKey: fmt.Sprintf("blocker-%d", index), Source: lifecycle.SourceFunction,
			Route: "blocker", Deadline: future,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		select {
		case <-blockerEntered:
		case <-time.After(time.Second):
			t.Fatal("blocking TaskChild did not occupy its slot")
		}
	}
	due := clock.Now()
	if err := kernel.Submit(context.Background(), Request{
		UID: "queued-deadline", LaneKey: "queued-deadline", Source: lifecycle.SourceFunction,
		Route: "deadline", Deadline: due,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case observed := <-deadlineEntered:
		t.Fatalf("queued deadline handler exceeded the four-slot bound: %+v", observed)
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
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	predecessorResult := make(chan error, 1)
	go func() {
		predecessorResult <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "same-lane-predecessor", LaneKey: "same-lane", Source: lifecycle.SourceFunction, Route: "predecessor",
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
			UID: "same-lane-deadline", LaneKey: "same-lane", Source: lifecycle.SourceFunction,
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
	blockerEntered := make(chan struct{}, lifecycle.TransientTaskSlots)
	permitPlan, err := lifecycle.NewSecretStoreLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	var prepareCalls atomic.Int32
	var abandonCalls atomic.Int32
	const capabilityID = "secret-store:queued-deadline"
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "capability" {
			return WorkPlan{
				NoResponse: true,
				Abandon: func() error {
					abandonCalls.Add(1)
					return nil
				},
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
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, io.Discard, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		if err := kernel.Submit(context.Background(), Request{
			UID: fmt.Sprintf("queued-capability-blocker-%d", index), LaneKey: fmt.Sprintf("queued-capability-blocker-%d", index),
			Source: lifecycle.SourceFunction, Route: "blocker",
		}); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		select {
		case <-blockerEntered:
		case <-time.After(time.Second):
			t.Fatal("blocking TaskChild did not occupy its slot")
		}
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
	if prepareCalls.Load() != 0 || abandonCalls.Load() != 1 {
		t.Fatalf("queued no-response capability prepare=%d abandon=%d, want 0/1", prepareCalls.Load(), abandonCalls.Load())
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
	blockerEntered := make(chan struct{}, lifecycle.TransientTaskSlots)
	var workCalls atomic.Int32
	var abandonCalls atomic.Int32
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		if route == "noncooperative" {
			return WorkPlan{
				Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
					workCalls.Add(1)
					return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
				}),
				Abandon: func() error {
					abandonCalls.Add(1)
					return nil
				},
			}, nil
		}
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			blockerEntered <- struct{}{}
			<-releaseBlockers
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	var output bytes.Buffer
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		if err := kernel.Submit(context.Background(), Request{
			UID: fmt.Sprintf("noncooperative-blocker-%d", index), LaneKey: fmt.Sprintf("noncooperative-blocker-%d", index),
			Source: lifecycle.SourceFunction, Route: "blocker",
		}); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		select {
		case <-blockerEntered:
		case <-time.After(time.Second):
			t.Fatal("blocking TaskChild did not occupy its slot")
		}
	}
	terminal := make(chan error, 1)
	go func() {
		terminal <- kernel.SubmitAndWait(context.Background(), Request{
			UID: "queued-noncooperative-deadline", LaneKey: "queued-noncooperative-deadline", Source: lifecycle.SourceFunction,
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
	if workCalls.Load() != 0 || abandonCalls.Load() != 1 {
		t.Fatalf("queued noncooperative work calls=%d abandon=%d, want 0/1", workCalls.Load(), abandonCalls.Load())
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
	entered := make(chan string, lifecycle.TransientTaskSlots)
	releaseWork := make(chan struct{})
	planner := plannerFunc(func(_ context.Context, route string, _ []string) (WorkPlan, error) {
		return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			entered <- route
			<-releaseWork
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})}, nil
	})
	writer := &firstHoldingFrameWriter{offered: make(chan []byte, 1), release: make(chan struct{})}
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, writer, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	deadline := clock.Now().Add(time.Second)
	terminals := make([]chan error, lifecycle.TransientTaskSlots)
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		terminals[index] = make(chan error, 1)
		request := Request{
			UID: fmt.Sprintf("mixed-retained-%d", index), LaneKey: fmt.Sprintf("mixed-retained-%d", index),
			Source: lifecycle.SourceFunction, Route: fmt.Sprintf("work-%d", index),
		}
		if index == 0 {
			request.Deadline = deadline
		}
		if err := kernel.submit(context.Background(), request, terminals[index]); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
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
	entered := make(chan string, lifecycle.TransientTaskSlots)
	permitPlan, err := lifecycle.NewSecretStoreLongLivedPlan(4096)
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
	kernel, run, admission, uids, tasks := newKernelWithClockFinalizerAndTimeout(t, planner, &output, clock, NoopRunFinalizer(), time.Second)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	deadline := clock.Now().Add(time.Second)
	terminals := make([]chan error, lifecycle.TransientTaskSlots)
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
		id := fmt.Sprintf("secret-store:background-%d", index)
		terminals[index] = make(chan error, 1)
		if err := kernel.submit(context.Background(), Request{
			UID: fmt.Sprintf("background-timeout-%d", index), LaneKey: id, Source: lifecycle.SourceJobManager,
			Route: "background-capability", Args: []string{id}, Deadline: deadline,
		}, terminals[index]); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < lifecycle.TransientTaskSlots; index++ {
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
	if cause := run.DirtyCause(); cause == nil || !strings.Contains(cause.Error(), "fourth background timeout saturated all task slots") {
		t.Fatalf("fourth background timeout dirty cause differs: %v", cause)
	}
	if count, saturated := tasks.RetainedTimeouts(); count != lifecycle.TransientTaskSlots || !saturated {
		t.Fatalf("background timeout census=(%d,%v), want (%d,true)", count, saturated, lifecycle.TransientTaskSlots)
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
	if terminalErr == nil || !strings.Contains(terminalErr.Error(), "fourth background timeout saturated all task slots") {
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
	requested := admission.RequestOrdinary(run.Generation(), lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1}, 1024)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	plan, err := lifecycle.NewSecretStoreLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
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
	seeded = make([]lifecycle.LongLivedPermit, 0, lifecycle.MaximumSteadyLongLivedRecords)
	for index := 0; index < lifecycle.MaximumSteadyLongLivedRecords; index++ {
		requested := admission.RequestOrdinary(run.Generation(), lifecycle.AdmissionLaneRef{Slot: uint16(index + 1), Generation: 1}, 2)
		if requested.Rejected != nil {
			t.Fatalf("seed admission %d: %v", index, requested.Rejected)
		}
		var grants [4]lifecycle.AdmissionGrant
		count, _, err := admission.TakeGrants(1, &grants)
		if err != nil || count != 1 || grants[0].Ref != requested.Ref {
			t.Fatalf("seed grant %d differs: count=%d grant=%+v err=%v", index, count, grants[0], err)
		}
		permit, err := tasks.IssueLongLivedPermit(
			admission, requested.Ref, lifecycle.ResourceIdentity{ID: fmt.Sprintf("seed:%d", index), Generation: 1}, steadyPlan,
		)
		if err != nil {
			t.Fatalf("seed permit %d: %v", index, err)
		}
		if _, err := admission.ReleaseOrdinary(requested.Ref); err != nil {
			t.Fatalf("seed ordinary release %d: %v", index, err)
		}
		seeded = append(seeded, permit)
	}
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
	if census := admission.Census(); census.ActiveRecords != lifecycle.MaximumSteadyLongLivedRecords || census.LongLivedRecords != lifecycle.MaximumSteadyLongLivedRecords || census.OrdinaryGranted != 0 {
		t.Fatalf("boundary operations left Admission ownership: %+v", census)
	}
	if census := tasks.LongLivedCensus(); census.Active != lifecycle.MaximumSteadyLongLivedRecords || census.Bytes != lifecycle.MaximumSteadyLongLivedRecords {
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
	requested := admission.RequestOrdinary(run.Generation(), lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1}, 1024)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	plan, err := lifecycle.NewJobLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
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
	frames, err := lifecycle.NewFrameOwner(io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCommandKernel(run, admission, uids, tasks, frames, lifecycle.RealClock{}, make(chan lifecycle.AdmissionGrant, 1), nil, nil, map[lifecycle.Source]Planner{
		lifecycle.SourceJobManager: stoppedKernelPlanner{}, lifecycle.SourceFunction: stoppedKernelPlanner{},
	}); err == nil {
		t.Fatal("Kernel accepted missing run finalizer")
	}
}

func TestKernelCannotReportQuiescentWithRetainedLongLivedPermit(t *testing.T) {
	kernel, run, admission, _, tasks := newKernelWithPlannerAndTimeout(t, stoppedKernelPlanner{}, time.Millisecond)
	requested := admission.RequestOrdinary(run.Generation(), lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1}, 1024)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant count=%d grant=%+v err=%v", count, grants[0], err)
	}
	permitPlan, err := lifecycle.NewJobLongLivedPlan(512)
	if err != nil {
		t.Fatal(err)
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
	if census := tasks.LongLivedCensus(); census.Active != 1 || census.Bytes != 512 || census.ExternalReserved != 1 {
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
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	if err := kernel.Submit(context.Background(), Request{
		UID: "cooperative-stop", LaneKey: "lane", Source: lifecycle.SourceFunction,
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
	frames, err := lifecycle.NewFrameOwner(io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	grants := make(chan lifecycle.AdmissionGrant, 1)
	kernel, err := NewCommandKernel(run, admission, uids, tasks, frames, lifecycle.RealClock{}, grants, nil, NoopRunFinalizer(), map[lifecycle.Source]Planner{
		lifecycle.SourceJobManager: stoppedKernelPlanner{},
		lifecycle.SourceFunction:   stoppedKernelPlanner{},
	})
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

func TestKernelAbandonsAcquiredPlanWhenQueuedOperationIsCancelled(t *testing.T) {
	started := make(chan struct{})
	abandoned := make(chan struct{}, 2)
	var abandonCount atomic.Int32
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
				Abandon: func() error {
					abandonCount.Add(1)
					abandoned <- struct{}{}
					return nil
				},
			}, nil
		default:
			return WorkPlan{}, errors.New("unexpected route")
		}
	})
	kernel, run, admission, uids, tasks := newKernelWithPlanner(t, planner)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	startKernelLoop(t, kernel)
	deadline := time.Now().Add(time.Minute)
	if err := kernel.Submit(context.Background(), Request{UID: "blocking", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "blocking", Deadline: deadline}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking operation did not start")
	}
	if err := kernel.Submit(context.Background(), Request{UID: "queued", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "queued", Deadline: deadline}); err != nil {
		t.Fatal(err)
	}
	if err := kernel.Cancel(context.Background(), "queued"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-abandoned:
	case <-time.After(time.Second):
		t.Fatal("queued plan lease was not abandoned")
	}
	if got := abandonCount.Load(); got != 1 {
		t.Fatalf("queued plan abandon count=%d, want 1", got)
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
	request := Request{UID: "self-fit", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"}
	if err := kernel.Submit(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var frame []byte
	select {
	case frame = <-writer.offered:
	case <-time.After(time.Second):
		t.Fatal("result did not reach held Write")
	}
	base, err := operationAdmissionBytes(request, WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)})
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
	blocker := admission.RequestOrdinary(run.Generation(), lifecycle.AdmissionLaneRef{Slot: lifecycle.MaximumAdmissionRecords, Generation: 1}, lifecycle.OrdinaryBudgetBytes-1024*1024)
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
	if err := kernel.Submit(context.Background(), Request{UID: "growth-cancel", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"}); err != nil {
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
		{UID: "f1", LaneKey: "f1", Source: lifecycle.SourceFunction, Route: "route"},
		{UID: "f2", LaneKey: "f2", Source: lifecycle.SourceFunction, Route: "route"},
	}
	results := make([]chan error, len(requests))
	for index, request := range requests {
		results[index] = make(chan error, 1)
		kernel.submissions[sourceIndex(request.Source)] <- submission{request: request, result: results[index]}
	}
	kernel.serviceSubmissions(4)
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
		kernel.submissions[sourceIndex(lifecycle.SourceFunction)] <- submission{
			request: Request{
				UID:     fmt.Sprintf("queued-%d", index),
				LaneKey: "lane",
				Source:  lifecycle.SourceFunction,
				Route:   "route",
			},
			result: results[index],
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
	kernel.blockedSubmissions[source] = submission{
		request: Request{UID: "ordinary", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"},
	}
	if !kernel.hasRunnableSubmissions() {
		t.Fatal("admission-blocked request was not exposed after admission closed")
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
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 9; index++ {
		request := Request{
			UID:     fmt.Sprintf("claim-%d", index),
			LaneKey: fmt.Sprintf("lane-%d", index),
			Source:  lifecycle.SourceFunction,
			Route:   "route",
		}
		if err := kernel.admit(request, nil); err != nil {
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

func TestKernelSustainedSubmissionRefillCannotStarveStop(t *testing.T) {
	planner := &refillingRejectPlanner{remaining: 100}
	kernel, run, _, _, _ := newKernelWithPlanner(t, planner)
	planner.kernel = kernel
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	planner.enqueue("refill-0")
	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.WaitShutdownStarted(ctx); err != nil {
		t.Fatal(err)
	}
	if planner.calls != 4 {
		t.Fatalf("stop was not serviced after one bounded submission turn: planner calls=%d", planner.calls)
	}
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestKernelSustainedSubmissionRefillCannotStarveDueDeadline(t *testing.T) {
	var output bytes.Buffer
	planner := &refillingRejectPlanner{remaining: 100, validUID: "deadline-probe"}
	kernel, run, _, _, _ := newKernelWithPlannerAndWriter(t, planner, &output)
	planner.kernel = kernel
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	if err := kernel.admit(Request{
		UID: "deadline-probe", LaneKey: "deadline-lane", Source: lifecycle.SourceFunction, Route: "route",
		Deadline: time.Now().Add(-time.Second),
	}, nil); err != nil {
		t.Fatal(err)
	}
	planner.enqueue("refill-0")
	kernel.Stop()
	startKernelLoop(t, kernel)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := kernel.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if planner.calls != 4 {
		t.Fatalf("deadline/stop event turn exceeded one submission quantum: planner calls=%d", planner.calls)
	}
	if !bytes.Contains(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN deadline-probe 504 application/json ")) {
		t.Fatalf("due deadline was starved or overwritten by shutdown: %q", output.Bytes())
	}
}

func TestKernelExternalSubmissionWaitsForRecordCapacityInSourceFIFO(t *testing.T) {
	kernel, run, admission, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	fillers := fillAdmissionRecordCapacity(t, admission, run.Generation())
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	source := sourceIndex(lifecycle.SourceFunction)
	kernel.submissions[source] <- submission{
		request: Request{UID: "first", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"},
		result:  firstResult,
	}
	kernel.submissions[source] <- submission{
		request: Request{UID: "second", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"},
		result:  secondResult,
	}

	kernel.serviceSubmissions(1)
	if !kernel.blockedSubmission[source] || len(kernel.submissions[source]) != 1 {
		t.Fatalf("capacity block did not retain exactly the source head: blocked=%v queued=%d", kernel.blockedSubmission[source], len(kernel.submissions[source]))
	}
	select {
	case err := <-firstResult:
		t.Fatalf("capacity-blocked submission completed early: %v", err)
	default:
	}

	if err := admission.CancelWaiting(fillers[0]); err != nil {
		t.Fatal(err)
	}
	kernel.serviceSubmissions(1)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}
	if kernel.blockedSubmission[source] || len(kernel.submissions[source]) != 1 {
		t.Fatalf("first retry did not preserve the later source item: blocked=%v queued=%d", kernel.blockedSubmission[source], len(kernel.submissions[source]))
	}
	select {
	case err := <-secondResult:
		t.Fatalf("later source submission completed before service: %v", err)
	default:
	}

	if err := admission.CancelWaiting(fillers[1]); err != nil {
		t.Fatal(err)
	}
	kernel.serviceSubmissions(1)
	if err := <-secondResult; err != nil {
		t.Fatal(err)
	}
	first := kernel.operations["first"]
	second := kernel.operations["second"]
	if first == nil || second == nil || first.ID >= second.ID {
		t.Fatalf("source FIFO operation order differs: first=%#v second=%#v", first, second)
	}
}

func TestKernelExternalSubmissionCapacityBlockFlushesAfterAdmissionClose(t *testing.T) {
	kernel, run, admission, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	fillAdmissionRecordCapacity(t, admission, run.Generation())
	result := make(chan error, 1)
	source := sourceIndex(lifecycle.SourceFunction)
	kernel.submissions[source] <- submission{
		request: Request{UID: "closing", LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"},
		result:  result,
	}
	kernel.serviceSubmissions(1)
	if !kernel.blockedSubmission[source] {
		t.Fatal("submission did not block at record capacity")
	}

	run.CloseAdmission()
	kernel.serviceSubmissions(1)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "admission closed") {
		t.Fatalf("closed admission result differs: %v", err)
	}
	if kernel.blockedSubmission[source] {
		t.Fatal("closed admission retained blocked submission")
	}
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

func fillAdmissionRecordCapacity(t *testing.T, admission *lifecycle.AdmissionLedger, runGeneration uint64) []lifecycle.AdmissionRef {
	t.Helper()
	refs := make([]lifecycle.AdmissionRef, 0, lifecycle.MaximumAdmissionRecords)
	for slot := 1; slot <= lifecycle.MaximumAdmissionRecords; slot++ {
		requested := admission.RequestOrdinary(runGeneration, lifecycle.AdmissionLaneRef{Slot: uint16(slot), Generation: 1}, 1)
		if requested.Rejected != nil {
			t.Fatalf("fill admission record %d: %v", slot, requested.Rejected)
		}
		refs = append(refs, requested.Ref)
	}
	if census := admission.Census(); census.ActiveRecords != lifecycle.MaximumAdmissionRecords || census.FreeRecords != 0 {
		t.Fatalf("record-capacity census differs: %#v", census)
	}
	return refs
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
		if err := generation.StartChild(lifecycle.TaskRef{Slot: uint8(index % lifecycle.TransientTaskSlots), Generation: uint64(index + 1)}); err != nil {
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
	return newKernelWithPlannerWriterFinalizerAndTimeout(t, planner, writer, NoopRunFinalizer(), timeout)
}

func newKernelWithPlannerWriterFinalizerAndTimeout(t *testing.T, planner Planner, writer io.Writer, finalizer RunFinalizer, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	return newKernelWithClockFinalizerAndTimeout(t, planner, writer, lifecycle.RealClock{}, finalizer, timeout)
}

func newKernelWithClockFinalizerAndTimeout(t *testing.T, planner Planner, writer io.Writer, clock lifecycle.Clock, finalizer RunFinalizer, timeout time.Duration) (*CommandKernel, *lifecycle.RunSupervisor, *lifecycle.AdmissionLedger, *lifecycle.UIDLedger, *lifecycle.TaskSupervisor) {
	t.Helper()
	run, err := lifecycle.NewRunSupervisor(1, clock, timeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.FinishShutdown() })
	uids := lifecycle.NewUIDLedger()
	admission := lifecycle.NewAdmissionLedger()
	frames, err := lifecycle.NewFrameOwner(writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := NewCommandKernel(run, admission, uids, tasks, frames, clock, make(chan lifecycle.AdmissionGrant, 1), nil, finalizer, map[lifecycle.Source]Planner{
		lifecycle.SourceJobManager: planner,
		lifecycle.SourceFunction:   planner,
	})
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

type plannerFunc func(context.Context, string, []string) (WorkPlan, error)

func (fn plannerFunc) Plan(request Request) (WorkPlan, error) {
	return fn(context.Background(), request.Route, request.Args)
}

type stoppedKernelPlanner struct{}

func (stoppedKernelPlanner) Plan(Request) (WorkPlan, error) {
	return WorkPlan{Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})}, nil
}

type refillingRejectPlanner struct {
	kernel    *CommandKernel
	remaining int
	next      int
	calls     int
	validUID  string
}

func (planner *refillingRejectPlanner) Plan(request Request) (WorkPlan, error) {
	if request.UID == planner.validUID && planner.validUID != "" {
		return WorkPlan{Work: lifecycle.FrameTaskWork(plannerPlanWork)}, nil
	}
	planner.calls++
	if planner.remaining > 0 {
		planner.remaining--
		planner.next++
		planner.enqueue(fmt.Sprintf("refill-%d", planner.next))
	}
	return WorkPlan{}, errors.New("test rejection")
}

func (planner *refillingRejectPlanner) enqueue(uid string) {
	planner.kernel.submissions[sourceIndex(lifecycle.SourceFunction)] <- submission{
		request: Request{UID: uid, LaneKey: "lane", Source: lifecycle.SourceFunction, Route: "route"},
		result:  make(chan error, 1),
	}
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
