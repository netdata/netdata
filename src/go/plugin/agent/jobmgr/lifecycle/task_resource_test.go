// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskSupervisorAcceptsStartsPublishesAndTransfersPreparedResource(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 7}, events: &events}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work: PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) {
			return prepared, nil
		}),
	})
	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Kind != TaskOutcomePreparedResource || completion.Err != nil)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 7}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Err != nil || ack.Kind != TaskActionAcceptStart)

	require.Error(t, supervisor.Release(ref))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionPublishResource}))

	acknowledgementCh := <-supervisor.AcknowledgementCh()
	require.False(t, acknowledgementCh.Err != nil || acknowledgementCh.Kind != TaskActionPublishResource)

	taken, err := supervisor.TakePublishedReadyResource(ref, 3, ready.identity)
	require.NoError(t, err)
	require.Same(t, ready, taken)

	_, takePublishedReadyResourceErr := supervisor.TakePublishedReadyResource(ref, 3, ready.identity)
	require.Error(t, takePublishedReadyResourceErr)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	got, want := events, []string{"accept-start", "publish"}
	require.Equal(t, want, got)
}

func TestTaskSupervisorRetainsPreparedResourceReturnedWithPrepareError(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	observer := &recordingRuntimeObserver{}

	require.NoError(t, supervisor.BindRuntimeObserver(observer))

	var events []string
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
	require.False(t, completion.Kind != TaskOutcomePreparedResource || !errors.Is(completion.Err, wantFailure))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.EqualValues(t, 1, observer.counter(RuntimeCounterResultsDisposed))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	got, want := events, []string{"dispose"}
	require.Equal(t, want, got)
}

func TestTaskSupervisorDoesNotRewriteActionCancellationAsStopping(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	entered := make(chan struct{})
	release := make(chan struct{})
	ready := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 1},
		events:   &events,
	}
	prepared := &recordingPreparedResource{
		identity:      ready.identity,
		ready:         ready,
		events:        &events,
		acceptEntered: entered,
		acceptGate:    release,
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work: PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) {
			return prepared, nil
		}),
	})
	require.NoError(t, (<-supervisor.CompletionCh()).Err)
	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart,
		ExpectedGeneration: 1,
	}))
	<-entered

	stopping := &StoppingRejection{Generation: 7}
	require.NoError(t, supervisor.CancelWithCause(ref, stopping))
	close(release)
	ack := <-supervisor.AcknowledgementCh()
	require.ErrorIs(t, ack.Err, context.Canceled)
	require.False(t, errors.As(ack.Err, new(*StoppingRejection)))

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 3, Kind: TaskActionTerminate,
	}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorRetainsReadyResourceAfterPublishFailureUntilAbort(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
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

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 1}))

	<-supervisor.AcknowledgementCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionPublishResource}))

	require.ErrorIs(t, (<-supervisor.AcknowledgementCh()).Err, wantFailure)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionDispose}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 5, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	got, want := events, []string{"accept-start", "publish", "abort-ready"}
	require.Equal(t, want, got)
}

func TestTaskSupervisorStopsAndFinalizesInitialReadyResource(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 3}, events: &events, panicAfterIdentity: true}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Kind != TaskOutcomeReadyResource || completion.Err != nil)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionStopResource}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionFinalizeResource}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	got, want := events, []string{"stop", "finalize"}
	require.Equal(t, want, got)
}

func TestTaskSupervisorFinalizesResourceOffLoop(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	finalizeEntered := make(chan struct{})
	finalizeRelease := make(chan struct{})
	ready := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 3}, events: &events,
		finalizeEntered: finalizeEntered, finalizeGate: finalizeRelease,
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionStopResource}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionFinalizeResource}))

	select {
	case <-finalizeEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "Finalize did not enter")
	}
	_, other := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction,
		Work: func(context.Context) (TaskOutcome, error) {
			return NoValueOutcome(), nil
		},
	})

	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != other || completion.Err != nil)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: other, Sequence: 2, Kind: TaskActionTerminate}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Ref != other || ack.Err != nil)

	require.NoError(t, supervisor.Release(other))

	close(finalizeRelease)

	acknowledgementCh := <-supervisor.AcknowledgementCh()
	require.False(t, acknowledgementCh.Ref != ref || acknowledgementCh.Err != nil)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))
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
			var events []string
			plan, contextErr := test.plan(&events)
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)

			completion := <-supervisor.CompletionCh()
			require.False(t, completion.Err != nil || completion.Kind != test.wantKind)

			require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, contextErr())

			require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.Release(ref))
		})
	}
}

func TestTaskSupervisorPreservesShutdownBudgetForResourceDisposal(t *testing.T) {
	run, err := NewRunSupervisor(1, RealClock{}, time.Minute)
	require.NoError(t, err)
	budget, err := run.BeginShutdown()
	require.NoError(t, err)
	t.Cleanup(func() { _ = run.FinishShutdown() })
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	plan, err := NewShutdownReadyResourceTaskPlan(SourceJobManager, budget, TransactionTaskPhases, ready, ready.identity)
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)

	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Err != nil || completion.Kind != TaskOutcomeReadyResource)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.False(t, ready.abortContextErr != nil || !ready.abortDeadline.Equal(budget.Deadline()))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorReturnsPendingInitialResourceOnTransferAwareCancellation(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 9}, events: &events}
	request, err := supervisor.Enqueue(
		TaskClassFrameworkControl,
		readyTaskPlan(t, SourceJobManager, time.Time{}, ready),
	)
	require.NoError(t, err)

	require.Error(t, supervisor.CancelPending(request))

	outcome, err := supervisor.CancelPendingOutcome(request)
	require.NoError(t, err)
	returned, ok := outcome.ReadyResource()
	require.False(t, !ok || returned != ready)
	require.EqualValues(t, 0, supervisor.Pending())

	_, cancelPendingOutcomeErr := supervisor.CancelPendingOutcome(request)
	require.Error(t, cancelPendingOutcomeErr)

}

func TestTaskSupervisorRetainsPreparedResourceWhenAcceptStartPanics(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events, panicAccept: true}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work:   PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) { return prepared, nil }),
	})
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 1}))

	require.ErrorIs(t, (<-supervisor.AcknowledgementCh()).Err, ErrTaskPanic)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.NotNil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.Error(t, supervisor.Release(ref))
}

func TestTaskSupervisorRetainsReadyResourceReturnedWithAcceptError(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	wantFailure := errors.New("start cleanup failed")
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	prepared := &recordingPreparedResource{identity: ready.identity, ready: ready, events: &events, acceptErr: wantFailure}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager,
		Work:   PreparedResourceTaskWork(func(context.Context) (PreparedResource, error) { return prepared, nil }),
	})
	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != ref || completion.Kind != TaskOutcomePreparedResource || completion.Err != nil)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionAcceptStart, ExpectedGeneration: 1}))

	ack := <-supervisor.AcknowledgementCh()
	require.ErrorIs(t, ack.Err, wantFailure)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionDispose}))

	ack = <-supervisor.AcknowledgementCh()
	require.Nil(t, ack.Err)

	got, want := events, []string{"accept-start", "abort-ready"}
	require.Equal(t, want, got)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))

	ack = <-supervisor.AcknowledgementCh()
	require.Nil(t, ack.Err)

	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorRetainsReadyResourceWhenAbortFails(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	wantFailure := errors.New("abort failed")
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events, abortErr: wantFailure}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	require.ErrorIs(t, (<-supervisor.AcknowledgementCh()).Err, wantFailure)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.NotNil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.Error(t, supervisor.Release(ref))
}

func newResourceTaskSupervisor(t *testing.T) *TaskSupervisor {
	t.Helper()
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	return supervisor
}

func readyTaskPlan(t *testing.T, source Source, deadline time.Time, resource ReadyResource) TaskPlan {
	t.Helper()
	identity, err := readyResourceIdentity(resource)
	require.NoError(t, err)
	plan, err := NewReadyResourceTaskPlan(source, deadline, TransactionTaskPhases, resource, identity)
	require.NoError(t, err)
	return plan
}

type recordingPreparedResource struct {
	identity          ResourceIdentity
	ready             ReadyResource
	events            *[]string
	consumed          bool
	panicAccept       bool
	acceptErr         error
	acceptEntered     chan struct{}
	acceptGate        <-chan struct{}
	disposeContextErr error
}

func (rpr *recordingPreparedResource) Identity() ResourceIdentity { return rpr.identity }

func (rpr *recordingPreparedResource) AcceptStart(ctx context.Context, _ uint64) (ReadyResource, error) {
	if rpr.panicAccept {
		panic("accept panic")
	}
	if rpr.acceptEntered != nil {
		close(rpr.acceptEntered)
	}
	if rpr.acceptGate != nil {
		<-rpr.acceptGate
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if rpr.consumed {
		return nil, errors.New("prepared resource consumed")
	}
	rpr.consumed = true
	*rpr.events = append(*rpr.events, "accept-start")
	return rpr.ready, rpr.acceptErr
}

func (rpr *recordingPreparedResource) Dispose(ctx context.Context) error {
	rpr.disposeContextErr = ctx.Err()
	if rpr.consumed {
		return errors.New("prepared resource consumed")
	}
	rpr.consumed = true
	*rpr.events = append(*rpr.events, "dispose")
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

func (rrr *recordingReadyResource) Identity() ResourceIdentity {
	rrr.identityCalls++
	if rrr.panicAfterIdentity && rrr.identityCalls > 1 {
		panic("ready identity called more than once")
	}
	return rrr.identity
}
func (rrr *recordingReadyResource) Publish() error {
	*rrr.events = append(*rrr.events, "publish")
	return rrr.publishErr
}
func (rrr *recordingReadyResource) AbortReady(ctx context.Context) error {
	rrr.abortContextErr = ctx.Err()
	rrr.abortDeadline, _ = ctx.Deadline()
	*rrr.events = append(*rrr.events, "abort-ready")
	return rrr.abortErr
}
func (rrr *recordingReadyResource) Stop(context.Context) error {
	*rrr.events = append(*rrr.events, "stop")
	return nil
}
func (rrr *recordingReadyResource) Finalize() error {
	*rrr.events = append(*rrr.events, "finalize")
	if rrr.finalizeEntered != nil {
		close(rrr.finalizeEntered)
	}
	if rrr.finalizeGate != nil {
		<-rrr.finalizeGate
	}
	return nil
}
