// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskSupervisorPreparationPanicReturnsTransferredOwnership(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	current := &recordingReadyResource{
		identity: ResourceIdentity{ID: "job", Generation: 1},
		events:   new([]string),
	}
	scope := ResourceTransactionScope{
		ID:        "job",
		Current:   current.identity,
		Successor: ResourceIdentity{ID: "job", Generation: 2},
	}
	permit := NewJobLongLivedPlan()
	plan, err := NewResourceTransactionPermitTaskPlan(
		SourceJobManager,
		time.Time{},
		TransactionTaskPhases,
		admission,
		admissionRef,
		current,
		scope,
		permit,
		func(context.Context, ReadyResource, ResourceTransactionScope, LongLivedPermit) (PreparedResourceTransaction, error) {
			panic("prepare transaction")
		},
	)
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)

	completion := <-supervisor.CompletionCh()
	require.ErrorIs(t, completion.Err, ErrTaskPanic)
	gotCurrent, err := supervisor.TakeDisposedResourceTransaction(ref, completion.Sequence, scope)
	require.NoError(t, err)
	require.NotNil(t, gotCurrent)
	require.Equal(t, scope.Current, gotCurrent.Identity())

	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 2, Kind: TaskActionDispose,
	}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 3, Kind: TaskActionTerminate,
	}))
	require.NoError(t, (<-supervisor.AcknowledgementCh()).Err)
	require.NoError(t, supervisor.Release(ref))
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())

	_, err = admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, err)
	census := admission.Census()
	require.Zero(t, census.ActiveRecords)
	require.Zero(t, census.OrdinaryBytes)
	require.Zero(t, census.LongLivedBytes)
}
