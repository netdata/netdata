// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskSupervisorPreparationPanicReturnsTransferredOwnership(t *testing.T) {
	tests := map[string]struct {
		plan func(
			*testing.T,
			*TaskSupervisor,
			*AdmissionLedger,
			AdmissionRef,
		) TaskPlan
		wantCurrent bool
	}{
		"prepared resource": {
			plan: func(
				t *testing.T,
				_ *TaskSupervisor,
				admission *AdmissionLedger,
				admissionRef AdmissionRef,
			) TaskPlan {
				permit, err := NewJobLongLivedPlan(40)
				require.NoError(t, err)
				plan, err := NewPreparedResourcePermitTaskPlan(
					SourceJobManager,
					time.Time{},
					TransactionTaskPhases,
					admission,
					admissionRef,
					ResourceIdentity{ID: "job", Generation: 1},
					permit,
					func(context.Context, LongLivedPermit) (
						PreparedResource,
						error,
					) {
						panic("prepare resource")
					},
				)
				require.NoError(t, err)
				return plan
			},
		},
		"prepared capability": {
			plan: func(
				t *testing.T,
				_ *TaskSupervisor,
				admission *AdmissionLedger,
				admissionRef AdmissionRef,
			) TaskPlan {
				permit, err := NewSecretStoreLongLivedPlan(40)
				require.NoError(t, err)
				plan, err := NewPreparedCapabilityPermitTaskPlan(
					SourceJobManager,
					time.Time{},
					TransactionTaskPhases,
					admission,
					admissionRef,
					ResourceIdentity{ID: "secret", Generation: 1},
					permit,
					func(context.Context, LongLivedPermit) (
						PreparedCapability,
						error,
					) {
						panic("prepare capability")
					},
				)
				require.NoError(t, err)
				return plan
			},
		},
		"resource transaction": {
			plan: func(
				t *testing.T,
				_ *TaskSupervisor,
				admission *AdmissionLedger,
				admissionRef AdmissionRef,
			) TaskPlan {
				current := &recordingReadyResource{
					identity: ResourceIdentity{ID: "job", Generation: 1},
					events:   new([]string),
				}
				scope := ResourceTransactionScope{
					ID:        "job",
					Current:   current.identity,
					Successor: ResourceIdentity{ID: "job", Generation: 2},
				}
				permit, err := NewJobLongLivedPlan(40)
				require.NoError(t, err)
				plan, err := NewResourceTransactionPermitTaskPlan(
					SourceJobManager,
					time.Time{},
					TransactionTaskPhases,
					admission,
					admissionRef,
					current,
					scope,
					permit,
					func(
						context.Context,
						ReadyResource,
						ResourceTransactionScope,
						LongLivedPermit,
					) (PreparedResourceTransaction, error) {
						panic("prepare transaction")
					},
				)
				require.NoError(t, err)
				return plan
			},
			wantCurrent: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			admission := NewAdmissionLedger()
			admissionRef := grantLongLivedTestAdmission(t, admission, 100)
			supervisor := newLongLivedTestSupervisor(t)
			plan := test.plan(t, supervisor, admission, admissionRef)
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)

			completion := <-supervisor.CompletionCh()
			require.ErrorIs(t, completion.Err, ErrTaskPanic)
			if test.wantCurrent {
				current, err := supervisor.TakeDisposedResourceTransaction(
					ref,
					completion.Sequence,
					ResourceTransactionScope{
						ID:        "job",
						Current:   ResourceIdentity{ID: "job", Generation: 1},
						Successor: ResourceIdentity{ID: "job", Generation: 2},
					},
				)
				require.NoError(t, err)
				require.False(t, current == nil ||
					current.Identity() !=
						(ResourceIdentity{ID: "job", Generation: 1}))

				require.NoError(t, supervisor.SendAction(TaskAction{
					Ref: ref, Sequence: 2, Kind: TaskActionDispose,
				}),
				)

				ack := <-supervisor.AcknowledgementCh()
				require.Nil(t, ack.Err)
			}
			sequence := uint8(2)
			if test.wantCurrent {
				sequence = 3
			}

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: sequence, Kind: TaskActionTerminate,
			}),
			)

			ack := <-supervisor.AcknowledgementCh()
			require.Nil(t, ack.Err)

			require.NoError(t, supervisor.Release(ref))

			require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())

			_, err := admission.ReleaseOrdinary(admissionRef)
			require.NoError(t, err)

			census := admission.Census()
			require.False(t, census.ActiveRecords != 0 || census.OrdinaryBytes != 0 || census.LongLivedBytes != 0)
		})
	}
}
