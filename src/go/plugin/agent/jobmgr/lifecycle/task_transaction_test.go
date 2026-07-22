// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
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
			scope:       ResourceTransactionScope{ID: "job"},
			disposition: ResourceTransactionUnchanged,
		},
		"remove current resource": {
			scope: ResourceTransactionScope{ID: "job", Current: ResourceIdentity{ID: "job", Generation: 1}},
			current: &recordingReadyResource{
				identity: ResourceIdentity{ID: "job", Generation: 1},
				events:   new([]string),
			},
			disposition: ResourceTransactionRemoved,
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
			prepared := &recordingPreparedResourceTransaction{
				scope: test.scope, current: test.current, applied: applied, events: &events,
			}
			plan, err := NewResourceTransactionTaskPlan(
				SourceJobManager,
				time.Time{},
				TransactionTaskPhases,
				test.current,
				test.scope,
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

			first := <-supervisor.CompletionCh()
			require.Equal(t, ref, first.Ref)
			require.EqualValues(t, 1, first.Sequence)
			require.Equal(t, TaskOutcomePreparedResourceTransaction, first.Kind)
			require.NoError(t, first.Err)
			require.NoError(t, supervisor.CancelWithCause(ref, &StoppingRejection{Generation: 7}))

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 2, Kind: TaskActionApplyResourceTransaction,
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
				Ref: ref, Sequence: 3, Kind: TaskActionEncodeWrite, UID: "tx", Expiry: 1,
			}),
			)

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionCleanup}))

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 5, Kind: TaskActionTerminate}))

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
	current := &recordingReadyResource{identity: ResourceIdentity{ID: "job", Generation: 7}, events: &events}
	scope := ResourceTransactionScope{ID: "job", Current: current.identity}
	prepared := &recordingPreparedResourceTransaction{scope: scope, current: current, events: &events}
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

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	restored, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.Same(t, current, restored)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	want := []string{"prepare", "dispose"}
	require.Equal(t, want, events)
}

func TestResourceTransactionPermitPlanRejectsPipelineReplacement(t *testing.T) {
	plan, err := NewPipelineLongLivedPlan([]string{"provider"})
	require.NoError(t, err)
	identity := ResourceIdentity{ID: "pipeline", Generation: 1}
	current := &recordingReadyResource{identity: identity, events: new([]string)}

	_, err = NewResourceTransactionPermitTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		current,
		ResourceTransactionScope{
			ID:        identity.ID,
			Current:   identity,
			Successor: ResourceIdentity{ID: identity.ID, Generation: 2},
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
}

func (rprt *recordingPreparedResourceTransaction) Scope() ResourceTransactionScope {
	return rprt.scope
}

func (rprt *recordingPreparedResourceTransaction) Apply(ctx context.Context) (AppliedResourceTransaction, error) {
	rprt.applyContextErr = ctx.Err()
	*rprt.events = append(*rprt.events, "apply")
	return rprt.applied, nil
}

func (rprt *recordingPreparedResourceTransaction) Dispose(context.Context) (ReadyResource, error) {
	*rprt.events = append(*rprt.events, "dispose")
	return rprt.current, nil
}
