// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
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
			scope: ResourceTransactionScope{
				ID:      "job",
				Current: ResourceIdentity{ID: "job", Generation: 1},
			},
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
			require.False(t, first.Ref != ref || first.Sequence != 1 ||
				first.Kind != TaskOutcomePreparedResourceTransaction ||
				first.Err != nil)
			require.NoError(t, supervisor.CancelWithCause(
				ref,
				&StoppingRejection{Generation: 7},
			))

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 2, Kind: TaskActionApplyResourceTransaction,
			}),
			)

			second := <-supervisor.CompletionCh()
			require.False(t, second.Ref != ref || second.Sequence != 2 ||
				second.Kind != TaskOutcomeAppliedResourceTransaction ||
				second.Err != nil)
			require.NoError(t, prepared.applyContextErr)
			disposition, current, err := supervisor.TakeAppliedResourceTransaction(ref, 2, test.scope)
			require.NoError(t, err)
			require.False(t, disposition != test.disposition || current != test.resulting)

			_, preflightResultErr := supervisor.PreflightResult(ref, "tx", 1)
			require.NoError(t, preflightResultErr)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 3, Kind: TaskActionEncodeWrite, UID: "tx", Expiry: 1,
			}),
			)

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 4, Kind: TaskActionCleanup,
			}),
			)

			require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 5, Kind: TaskActionTerminate,
			}),
			)

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
		identity: ResourceIdentity{ID: "job", Generation: 7},
		events:   &events,
	}
	scope := ResourceTransactionScope{
		ID: "job", Current: current.identity,
	}
	prepared := &recordingPreparedResourceTransaction{
		scope: scope, current: current, events: &events,
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
		Ref: ref, Sequence: 2, Kind: TaskActionDispose,
	}),
	)

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	restored, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
	require.NoError(t, err)
	require.Same(t, current, restored)

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 3, Kind: TaskActionTerminate,
	}),
	)

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	want := []string{"prepare", "dispose"}
	require.Equal(t, want, events)
}

func TestTaskSupervisorRejectsSecondSteadyPipelineTransaction(t *testing.T) {
	tests := map[string]struct {
		permitPlan func([]string) (LongLivedPlan, error)
	}{
		"pipeline": {permitPlan: NewPipelineLongLivedPlan},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			admission := NewAdmissionLedger()
			supervisor := newResourceTaskSupervisor(t)
			var grants [4]AdmissionGrant
			seedPlan, err := test.permitPlan([]string{"provider"})
			require.NoError(t, err)
			seedAdmission := admission.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 1, Generation: 1},
				seedPlan.Bytes()+1,
			)
			require.Nil(t, seedAdmission.Rejected)
			count, _, err := admission.TakeGrants(1, &grants)
			require.False(t, err != nil || count != 1)
			seed, err := supervisor.IssueLongLivedPermit(
				admission,
				seedAdmission.Ref,
				ResourceIdentity{ID: "seed", Generation: 1},
				seedPlan,
			)
			require.NoError(t, err)

			_, releaseOrdinaryErr := admission.ReleaseOrdinary(seedAdmission.Ref)
			require.NoError(t, releaseOrdinaryErr)

			permitPlan, err := test.permitPlan([]string{"provider"})
			require.NoError(t, err)
			transactionAdmission := admission.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 2, Generation: 1},
				permitPlan.Bytes()+1,
			)
			require.Nil(t, transactionAdmission.Rejected)
			count, _, err = admission.TakeGrants(1, &grants)
			require.False(t, err != nil || count != 1)
			scope := ResourceTransactionScope{
				ID:        "successor",
				Successor: ResourceIdentity{ID: "successor", Generation: 1},
			}
			var transactionRan atomic.Bool
			plan, err := NewResourceTransactionPermitTaskPlan(
				SourceJobManager,
				time.Time{},
				TransactionTaskPhases,
				admission,
				transactionAdmission.Ref,
				nil,
				scope,
				permitPlan,
				func(
					context.Context,
					ReadyResource,
					ResourceTransactionScope,
					LongLivedPermit,
				) (PreparedResourceTransaction, error) {
					transactionRan.Store(true)
					return nil, errors.New("capacity-rejected transaction ran")
				},
			)
			require.NoError(t, err)
			requestRef, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
			require.NoError(t, err)
			var started [TaskStartServiceQuantum]TaskStart
			count, _, err = supervisor.Dispatch(context.Background(), 1, &started)
			require.False(t, err != nil ||
				count != 1 ||
				started[0].Request != requestRef ||
				!errors.Is(started[0].Err, ErrLongLivedRecordCapacity))
			require.EqualValues(t, TaskOutcomeNone, started[0].Outcome.Kind())
			require.False(t, transactionRan.Load())
			require.False(t, supervisor.Active() != 0 || supervisor.Pending() != 0)

			_, releaseOrdinaryErr2 := admission.ReleaseOrdinary(transactionAdmission.Ref)
			require.NoError(t, releaseOrdinaryErr2)

			require.NoError(t, seed.AbortUnused())

			census := admission.Census()
			require.False(t, census.ActiveRecords != 0 || census.OrdinaryBytes != 0 || census.LongLivedBytes != 0)
		})
	}
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

func (rprt *recordingPreparedResourceTransaction) Apply(
	ctx context.Context,
) (AppliedResourceTransaction, error) {
	rprt.applyContextErr = ctx.Err()
	*rprt.events = append(*rprt.events, "apply")
	return rprt.applied, nil
}

func (rprt *recordingPreparedResourceTransaction) Dispose(
	context.Context,
) (ReadyResource, error) {
	*rprt.events = append(*rprt.events, "dispose")
	return rprt.current, nil
}
