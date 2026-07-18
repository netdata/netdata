// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestTaskSupervisorAcceptsStartsPublishesAndTransfersPreparedResource(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 7}, events: &events}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work: PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) {
			return prepared, nil
		}),
	})
	completion := <-supervisor.CompletionCh()
	if completion.Kind != TaskOutcomePreparedResource || completion.Err != nil {
		t.Fatalf("completion=%#v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 7}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil || ack.Kind != TaskActionAcceptStart {
		t.Fatalf("accept/start acknowledgement=%#v", ack)
	}
	if err := supervisor.Release(ref); err == nil {
		t.Fatal("slot released while ready resource was present")
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionPublishResource}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil || ack.Kind != TaskActionPublishResource {
		t.Fatalf("publish acknowledgement=%#v", ack)
	}
	taken, err := supervisor.TakePublishedReadyResource(ref, 3, ready.identity)
	if err != nil {
		t.Fatal(err)
	}
	if taken != ready {
		t.Fatalf("transferred resource=%T %p, want %p", taken, taken, ready)
	}
	if _, err := supervisor.TakePublishedReadyResource(ref, 3, ready.identity); err == nil {
		t.Fatal("duplicate ready-resource transfer succeeded")
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"accept-start", "publish"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestTaskSupervisorRetainsPreparedResourceReturnedWithPrepareError(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	wantFailure := errors.New("construction cleanup failed")
	prepared := &recordingPreparedResource{
		identity: ResourceIdentity{ID: "pipeline", Generation: 1}, events: &events,
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work: PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) {
			return prepared, wantFailure
		}),
	})
	completion := <-supervisor.CompletionCh()
	if completion.Kind != TaskOutcomePreparedResource || !errors.Is(completion.Err, wantFailure) {
		t.Fatalf("completion=%#v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"dispose"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestTaskSupervisorRetainsReadyResourceAfterPublishFailureUntilAbort(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	wantFailure := errors.New("publish failed")
	ready := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events, publishErr: wantFailure,
	}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work: PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) {
			return prepared, nil
		}),
	})
	<-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	<-supervisor.AcknowledgementCh()
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionPublishResource}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); !errors.Is(ack.Err, wantFailure) {
		t.Fatalf("publish acknowledgement=%#v want=%v", ack, wantFailure)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 5, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"accept-start", "publish", "abort-ready"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestTaskSupervisorStopsAndFinalizesInitialReadyResource(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 3}, events: &events, panicAfterIdentity: true}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	completion := <-supervisor.CompletionCh()
	if completion.Kind != TaskOutcomeReadyResource || completion.Err != nil {
		t.Fatalf("completion=%#v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionStopResource}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionFinalizeResource}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"stop", "finalize"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestTaskSupervisorFinalizesResourceOffLoop(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	finalizeEntered := make(chan struct{})
	finalizeRelease := make(chan struct{})
	ready := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 3}, events: &events,
		finalizeEntered: finalizeEntered, finalizeGate: finalizeRelease,
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	<-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionStopResource}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionFinalizeResource}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finalizeEntered:
	case <-time.After(time.Second):
		t.Fatal("Finalize did not enter")
	}
	_, other := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction,
		Work: func(context.Context) (TaskOutcome, error) {
			return NoValueOutcome(), nil
		},
	})
	if completion := <-supervisor.CompletionCh(); completion.Ref != other || completion.Err != nil {
		t.Fatalf("unrelated completion=%#v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: other, Sequence: 2, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Ref != other || ack.Err != nil {
		t.Fatalf("unrelated acknowledgement=%#v", ack)
	}
	if err := supervisor.Release(other); err != nil {
		t.Fatal(err)
	}
	close(finalizeRelease)
	if ack := <-supervisor.AcknowledgementCh(); ack.Ref != ref || ack.Err != nil {
		t.Fatalf("finalize acknowledgement=%#v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorDisposesResourcesWithoutExpiredWorkContext(t *testing.T) {
	tests := map[string]struct {
		plan     func(*[]string) (TaskPlan, func() error)
		wantKind TaskOutcomeKind
	}{
		"prepared": {
			plan: func(events *[]string) (TaskPlan, func() error) {
				prepared := &recordingPreparedResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: events}
				return TaskPlan{
					Source: SourceJobManager, Deadline: time.Now().Add(-time.Second),
					Work: PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) { return prepared, nil }),
				}, func() error { return prepared.disposeContextErr }
			},
			wantKind: TaskOutcomePreparedResource,
		},
		"ready": {
			plan: func(events *[]string) (TaskPlan, func() error) {
				ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: events}
				return readyTaskPlan(t, SourceJobManager, time.Now().Add(-time.Second), ready), func() error {
					return ready.abortContextErr
				}
			},
			wantKind: TaskOutcomeReadyResource,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			supervisor := newResourceTaskSupervisor(t)
			events := []string{}
			plan, contextErr := test.plan(&events)
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)
			if completion := <-supervisor.CompletionCh(); completion.Err != nil || completion.Kind != test.wantKind {
				t.Fatalf("completion=%#v", completion)
			}
			if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
				t.Fatal(err)
			}
			if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
				t.Fatal(ack.Err)
			}
			if err := contextErr(); err != nil {
				t.Fatalf("resource disposal inherited expired work context: %v", err)
			}
			if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
				t.Fatal(err)
			}
			if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
				t.Fatal(ack.Err)
			}
			if err := supervisor.Release(ref); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTaskSupervisorPreservesShutdownBudgetForResourceDisposal(t *testing.T) {
	run, err := NewRunSupervisor(1, RealClock{}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := run.BeginShutdown()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.FinishShutdown() })
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	plan, err := NewShutdownReadyResourceTaskPlan(SourceJobManager, budget, TransactionTaskPhases, ready, ready.identity)
	if err != nil {
		t.Fatal(err)
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)
	if completion := <-supervisor.CompletionCh(); completion.Err != nil || completion.Kind != TaskOutcomeReadyResource {
		t.Fatalf("completion=%#v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if ready.abortContextErr != nil || !ready.abortDeadline.Equal(budget.Deadline()) {
		t.Fatalf("shutdown disposal context differs: err=%v deadline=%s want=%s", ready.abortContextErr, ready.abortDeadline, budget.Deadline())
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorReturnsPendingInitialResourceOnTransferAwareCancellation(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 9}, events: &events}
	request, err := supervisor.Enqueue(
		TaskClassFrameworkControl,
		readyTaskPlan(t, SourceJobManager, time.Time{}, ready),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.CancelPending(request); err == nil {
		t.Fatal("non-transfer-aware cancellation accepted a pending resource")
	}
	outcome, err := supervisor.CancelPendingOutcome(request)
	if err != nil {
		t.Fatal(err)
	}
	returned, ok := outcome.ReadyResource()
	if !ok || returned != ready {
		t.Fatalf("returned resource=%T %p ok=%v want=%p", returned, returned, ok, ready)
	}
	if supervisor.Pending() != 0 {
		t.Fatalf("pending=%d, want zero", supervisor.Pending())
	}
	if _, err := supervisor.CancelPendingOutcome(request); err == nil {
		t.Fatal("stale transfer-aware cancellation succeeded")
	}
}

func TestTaskSupervisorRetainsPreparedResourceWhenAcceptStartPanics(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events, panicAccept: true}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work:   PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) { return prepared, nil }),
	})
	<-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); !errors.Is(ack.Err, ErrTaskPanic) {
		t.Fatalf("accept/start panic acknowledgement=%#v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err == nil {
		t.Fatal("termination acknowledged despite retained prepared resource")
	}
	if err := supervisor.Release(ref); err == nil {
		t.Fatal("slot released after prepared-resource panic")
	}
}

func TestTaskSupervisorRetainsReadyResourceReturnedWithAcceptError(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	wantFailure := errors.New("start cleanup failed")
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events, acceptErr: wantFailure}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work:   PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) { return prepared, nil }),
	})
	completion := <-supervisor.CompletionCh()
	if completion.Ref != ref || completion.Kind != TaskOutcomePreparedResource || completion.Err != nil {
		t.Fatalf("completion=%+v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if !errors.Is(ack.Err, wantFailure) {
		t.Fatalf("accept acknowledgement=%+v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	ack = <-supervisor.AcknowledgementCh()
	if ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if got, want := events, []string{"accept-start", "abort-ready"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack = <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorRetainsReadyResourceWhenAbortFails(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
	wantFailure := errors.New("abort failed")
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events, abortErr: wantFailure}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	<-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); !errors.Is(ack.Err, wantFailure) {
		t.Fatalf("abort acknowledgement=%#v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err == nil {
		t.Fatal("termination acknowledged despite retained ready resource")
	}
	if err := supervisor.Release(ref); err == nil {
		t.Fatal("slot released after ready-resource abort failure")
	}
}

func newResourceTaskSupervisor(t *testing.T) *TaskSupervisor {
	t.Helper()
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
	return supervisor
}

func readyTaskPlan(t *testing.T, source Source, deadline time.Time, resource ReadyResource) TaskPlan {
	t.Helper()
	identity, err := readyResourceIdentity(resource)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewReadyResourceTaskPlan(source, deadline, TransactionTaskPhases, resource, identity)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

type recordingPreparedResource struct {
	identity          ResourceIdentity
	ready             ReadyResource
	events            *[]string
	consumed          bool
	panicAccept       bool
	acceptErr         error
	disposeContextErr error
}

func (resource *recordingPreparedResource) Identity() ResourceIdentity { return resource.identity }

func (resource *recordingPreparedResource) AcceptStart(context.Context, uint64) (ReadyResource, error) {
	if resource.panicAccept {
		panic("accept panic")
	}
	if resource.consumed {
		return nil, errors.New("prepared resource consumed")
	}
	resource.consumed = true
	*resource.events = append(*resource.events, "accept-start")
	return resource.ready, resource.acceptErr
}

func (resource *recordingPreparedResource) Dispose(ctx context.Context) error {
	resource.disposeContextErr = ctx.Err()
	if resource.consumed {
		return errors.New("prepared resource consumed")
	}
	resource.consumed = true
	*resource.events = append(*resource.events, "dispose")
	return nil
}

type recordingReadyResource struct {
	identity           ResourceIdentity
	events             *[]string
	publishErr         error
	abortErr           error
	abortContextErr    error
	abortDeadline      time.Time
	finalizeEntered    chan struct{}
	finalizeGate       <-chan struct{}
	identityCalls      int
	panicAfterIdentity bool
}

func (resource *recordingReadyResource) Identity() ResourceIdentity {
	resource.identityCalls++
	if resource.panicAfterIdentity && resource.identityCalls > 1 {
		panic("ready identity called more than once")
	}
	return resource.identity
}
func (resource *recordingReadyResource) Publish() error {
	*resource.events = append(*resource.events, "publish")
	return resource.publishErr
}
func (resource *recordingReadyResource) AbortReady(ctx context.Context) error {
	resource.abortContextErr = ctx.Err()
	resource.abortDeadline, _ = ctx.Deadline()
	*resource.events = append(*resource.events, "abort-ready")
	return resource.abortErr
}
func (resource *recordingReadyResource) Stop(context.Context) error {
	*resource.events = append(*resource.events, "stop")
	return nil
}
func (resource *recordingReadyResource) Finalize() error {
	*resource.events = append(*resource.events, "finalize")
	if resource.finalizeEntered != nil {
		close(resource.finalizeEntered)
	}
	if resource.finalizeGate != nil {
		<-resource.finalizeGate
	}
	return nil
}
