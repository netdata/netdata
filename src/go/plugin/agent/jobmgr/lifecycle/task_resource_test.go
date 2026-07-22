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

func TestTaskSupervisorStopsAndFinalizesInitialReadyResource(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{
		identity:           ResourceIdentity{ID: "job", Generation: 3},
		events:             &events,
		panicAfterIdentity: true,
	}
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

func TestTaskSupervisorStopsWithNonCancellableActionContext(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 3}, events: &events}
	_, ref := enqueueAndDispatchTask(t, supervisor, readyTaskPlan(t, SourceJobManager, time.Time{}, ready))
	require.NoError(t, (<-supervisor.CompletionCh()).Err)
	require.NoError(t, supervisor.CancelWithCause(ref, &StoppingRejection{Generation: 7}))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionStopResource}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, ready.stopContextErr)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionFinalizeResource}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, supervisor.Release(ref))
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
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	ready := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 1}, events: &events}
	plan := readyTaskPlan(t, SourceJobManager, time.Now().Add(-time.Second), ready)
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)

	completion := <-supervisor.CompletionCh()
	require.NoError(t, completion.Err)
	require.Equal(t, TaskOutcomeReadyResource, completion.Kind)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, ready.abortContextErr)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, supervisor.Release(ref))
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
	plan, err := NewShutdownReadyResourceTaskPlan(
		SourceJobManager,
		budget,
		TransactionTaskPhases,
		ready,
		ready.identity,
	)
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

func TestTaskSupervisorRetainsReadyResourceWhenAbortFails(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	wantFailure := errors.New("abort failed")
	ready := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 1},
		events:   &events,
		abortErr: wantFailure,
	}
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
	plan := TaskPlan{
		Source:              source,
		Deadline:            deadline,
		MaxPhaseTransitions: TransactionTaskPhases,
		initialReady:        resource,
		initialIdentity:     identity,
		drainDependent:      true,
	}
	err = plan.Validate()
	require.NoError(t, err)
	return plan
}

type recordingReadyResource struct {
	identity           ResourceIdentity
	events             *[]string
	publishErr         error
	abortErr           error
	abortContextErr    error
	abortDeadline      time.Time
	stopContextErr     error
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
func (rrr *recordingReadyResource) Stop(ctx context.Context) error {
	rrr.stopContextErr = ctx.Err()
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
