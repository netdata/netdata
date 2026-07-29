// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskSupervisorRunsSealedResourceTransactionInOriginalSlot(t *testing.T) {
	tests := map[string]struct {
		scope       ResourceTransactionScope
		current     ReadyResource
		disposition ResourceTransactionDisposition
		resulting   ReadyResource
	}{
		"graph-only command": {
			scope: ResourceTransactionScope{
				ID: "job",
			},
			disposition: ResourceTransactionUnchanged,
		},
		"remove current resource": {
			scope: ResourceTransactionScope{
				ID: "job",
				Current: ResourceIdentity{
					ID:         "job",
					Generation: 1,
				},
			},
			current: &recordingReadyResource{
				identity: ResourceIdentity{
					ID:         "job",
					Generation: 1,
				},
				events: new([]string),
			},
			disposition: ResourceTransactionRemoved,
		},
		"install projection without run permit": {
			scope: ResourceTransactionScope{
				ID: "projection",
				Successor: ResourceIdentity{
					ID:         "projection",
					Generation: 1,
				},
			},
			disposition: ResourceTransactionInstalled,
			resulting: &recordingReadyResource{
				identity: ResourceIdentity{
					ID:         "projection",
					Generation: 1,
				},
				events: new([]string),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			supervisor := newResourceTaskSupervisor(t)
			var events []string
			result, err := NewSealedResult(200, "application/json", []byte(`{"ok":true}`))
			require.NoError(t, err)
			applied, err := NewAppliedResourceTransaction(
				test.scope,
				test.disposition,
				test.resulting,
				result,
				func() error {
					events = append(events, "cleanup")
					return nil
				},
			)
			require.NoError(t, err)
			require.EqualValues(t, 200, applied.ResultStatus())
			prepared := &recordingPreparedResourceTransaction{
				scope:   test.scope,
				current: test.current,
				applied: applied,
				events:  &events,
			}
			plan, err := NewResourceTransactionTaskPlan(
				SourceJobManager,
				time.Time{},
				TransactionTaskPhases,
				test.current,
				test.scope,
				func(
					_ context.Context,
					_ ReadyResource,
					_ ResourceTransactionScope,
					permit LongLivedPermit,
				) (PreparedResourceTransaction, error) {
					require.False(t, permit.Valid())
					events = append(events, "prepare")
					return prepared, nil
				},
			)
			require.NoError(t, err)
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)

			first := <-supervisor.CompletionCh()
			require.Equal(t, ref, first.Ref)
			require.EqualValues(t, 1, first.Sequence)
			require.Equal(t, TaskOutcomePreparedResourceTransaction, first.Kind)
			require.NoError(t, first.Err)
			require.NoError(t, supervisor.CancelWithCause(ref, &StoppingRejection{
				Generation: 7,
			}))

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref:      ref,
				Sequence: 2,
				Kind:     TaskActionApplyResourceTransaction,
			}),
			)

			second := <-supervisor.CompletionCh()
			require.Equal(t, ref, second.Ref)
			require.EqualValues(t, 2, second.Sequence)
			require.Equal(t, TaskOutcomeAppliedResourceTransaction, second.Kind)
			require.NoError(t, second.Err)
			require.NoError(t, prepared.applyContextErr)
			disposition, current, err := supervisor.TakeAppliedResourceTransaction(ref, 2, test.scope)
			require.NoError(t, err)
			require.Equal(t, test.disposition, disposition)
			require.Equal(t, test.resulting, current)

			preflightResultErr := supervisor.PreflightResult(ref, "tx", 1)
			require.NoError(t, preflightResultErr)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref:      ref,
				Sequence: 3,
				Kind:     TaskActionEncodeWrite,
				UID:      "tx",
				Expiry:   1,
			}),
			)

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref:      ref,
				Sequence: 4,
				Kind:     TaskActionCleanup,
			}))

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref:      ref,
				Sequence: 5,
				Kind:     TaskActionTerminate,
			}))

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.Release(ref))

			want := []string{"prepare", "apply", "cleanup"}
			require.Equal(t, want, events)
		})
	}
}

func TestTaskSupervisorDisposesPreparedTransactionAndRestoresCurrent(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	current := &recordingReadyResource{
		identity: ResourceIdentity{
			ID:         "job",
			Generation: 7,
		},
		events: &events,
	}
	scope := ResourceTransactionScope{
		ID:      "job",
		Current: current.identity,
	}
	prepared := &recordingPreparedResourceTransaction{
		scope:   scope,
		current: current,
		events:  &events,
	}
	plan, err := NewResourceTransactionTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		current,
		scope,
		func(
			context.Context,
			ReadyResource,
			ResourceTransactionScope,
			LongLivedPermit,
		) (PreparedResourceTransaction, error) {
			events = append(events, "prepare")
			return prepared, nil
		},
	)
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref:      ref,
		Sequence: 2,
		Kind:     TaskActionDispose,
	}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	restored, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.Same(t, current, restored)

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref:      ref,
		Sequence: 3,
		Kind:     TaskActionTerminate,
	}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	want := []string{"prepare", "dispose"}
	require.Equal(t, want, events)
}

func TestTaskSupervisorPreservesDisposedCurrentAlongsideError(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	failure := errors.New("dispose failed")
	current := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 7},
		events:   new([]string),
	}
	scope := ResourceTransactionScope{ID: "job", Current: current.identity}
	prepared := &recordingPreparedResourceTransaction{
		scope:      scope,
		current:    current,
		events:     new([]string),
		disposeErr: failure,
	}
	plan, err := NewResourceTransactionTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		current,
		scope,
		func(
			context.Context,
			ReadyResource,
			ResourceTransactionScope,
			LongLivedPermit,
		) (PreparedResourceTransaction, error) {
			return prepared, nil
		},
	)
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref:      ref,
		Sequence: 2,
		Kind:     TaskActionDispose,
	}))
	require.ErrorIs(t, (<-supervisor.AcknowledgementCh()).Err, failure)

	restored, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.Same(t, current, restored)
}

func TestTaskSupervisorPreservesAppliedOwnershipAlongsideError(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	failure := errors.New("apply failed after ownership changed")
	owned := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 1},
		events:   new([]string),
	}
	scope := ResourceTransactionScope{
		ID:      "job",
		Current: owned.identity,
	}
	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	applied, err := NewAppliedResourceTransaction(
		scope,
		ResourceTransactionUnchanged,
		owned,
		result,
		func() error { return nil },
	)
	require.NoError(t, err)
	prepared := &recordingPreparedResourceTransaction{
		scope:    scope,
		applied:  applied,
		events:   new([]string),
		applyErr: failure,
	}
	plan, err := NewResourceTransactionTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		owned,
		scope,
		func(
			context.Context,
			ReadyResource,
			ResourceTransactionScope,
			LongLivedPermit,
		) (PreparedResourceTransaction, error) {
			return prepared, nil
		},
	)
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref:      ref,
		Sequence: 2,
		Kind:     TaskActionApplyResourceTransaction,
	}))
	completion := <-supervisor.CompletionCh()
	require.ErrorIs(t, completion.Err, failure)
	require.Equal(t, TaskOutcomeAppliedResourceTransaction, completion.Kind)

	disposition, current, err := supervisor.TakeAppliedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.Equal(t, ResourceTransactionUnchanged, disposition)
	require.Same(t, owned, current)
}

func TestTaskSupervisorParksStartFailureOutsideRunnableQueue(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "job", Generation: 1}
	scope := ResourceTransactionScope{ID: owner.ID, Successor: owner}
	plan, err := NewResourceTransactionPermitTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		nil,
		scope,
		NewJobLongLivedPlan(),
		func(
			context.Context,
			ReadyResource,
			ResourceTransactionScope,
			LongLivedPermit,
		) (PreparedResourceTransaction, error) {
			return nil, nil
		},
	)
	require.NoError(t, err)
	first, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
	require.NoError(t, err)
	second, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
	require.NoError(t, err)

	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), 2, &started)
	require.Error(t, err)
	require.Equal(t, 1, count)
	require.False(t, more)
	require.Zero(t, supervisor.Pending())

	_, err = supervisor.CancelPendingOutcome(second)
	require.NoError(t, err)
	_, err = supervisor.CancelPendingOutcome(first)
	require.Error(t, err)

	completion := <-supervisor.CompletionCh()
	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref:      completion.Ref,
		Sequence: completion.Sequence + 1,
		Kind:     TaskActionTerminate,
	}))
	<-supervisor.AcknowledgementCh()
	require.NoError(t, supervisor.Release(completion.Ref))
}

func TestResourceTransactionPermitPlanRejectsPipelineReplacement(t *testing.T) {
	plan, err := NewPipelineLongLivedPlan([]string{"provider"})
	require.NoError(t, err)
	identity := ResourceIdentity{
		ID:         "pipeline",
		Generation: 1,
	}
	current := &recordingReadyResource{
		identity: identity,
		events:   new([]string),
	}

	_, err = NewResourceTransactionPermitTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		current,
		ResourceTransactionScope{
			ID:      identity.ID,
			Current: identity,
			Successor: ResourceIdentity{
				ID:         identity.ID,
				Generation: 2,
			},
		},
		plan,
		func(context.Context, ReadyResource, ResourceTransactionScope, LongLivedPermit) (PreparedResourceTransaction, error) {
			return nil, nil
		},
	)
	require.Error(t, err)
}

type recordingPreparedResourceTransaction struct {
	scope           ResourceTransactionScope
	current         ReadyResource
	applied         AppliedResourceTransaction
	events          *[]string
	applyContextErr error
	applyErr        error
	disposeErr      error
}

func (rprt *recordingPreparedResourceTransaction) Scope() ResourceTransactionScope {
	return rprt.scope
}

func (rprt *recordingPreparedResourceTransaction) Apply(ctx context.Context) (AppliedResourceTransaction, error) {
	rprt.applyContextErr = ctx.Err()
	*rprt.events = append(*rprt.events, "apply")
	return rprt.applied, rprt.applyErr
}

func (rprt *recordingPreparedResourceTransaction) Dispose(context.Context) (ReadyResource, error) {
	*rprt.events = append(*rprt.events, "dispose")
	return rprt.current, rprt.disposeErr
}
