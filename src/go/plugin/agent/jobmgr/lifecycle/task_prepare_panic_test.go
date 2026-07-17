// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
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
				if err != nil {
					t.Fatal(err)
				}
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
				if err != nil {
					t.Fatal(err)
				}
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
				if err != nil {
					t.Fatal(err)
				}
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
				if err != nil {
					t.Fatal(err)
				}
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
				if err != nil {
					t.Fatal(err)
				}
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
				if err != nil {
					t.Fatal(err)
				}
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
			if !errors.Is(completion.Err, ErrTaskPanic) {
				t.Fatalf("preparation panic completion=%+v", completion)
			}
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
				if err != nil {
					t.Fatal(err)
				}
				if current == nil ||
					current.Identity() !=
						(ResourceIdentity{ID: "job", Generation: 1}) {
					t.Fatalf("restored current=%T %#v", current, current)
				}
				if err := supervisor.SendAction(TaskAction{
					Ref: ref, Sequence: 2, Kind: TaskActionDispose,
				}); err != nil {
					t.Fatal(err)
				}
				if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
					t.Fatal(ack.Err)
				}
			}
			sequence := uint8(2)
			if test.wantCurrent {
				sequence = 3
			}
			if err := supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: sequence, Kind: TaskActionTerminate,
			}); err != nil {
				t.Fatal(err)
			}
			if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
				t.Fatal(ack.Err)
			}
			if err := supervisor.Release(ref); err != nil {
				t.Fatal(err)
			}
			if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
				t.Fatalf("preparation panic retained permit: %+v", census)
			}
			if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
				t.Fatal(err)
			}
			if census := admission.Census(); census.ActiveRecords != 0 ||
				census.OrdinaryBytes != 0 ||
				census.LongLivedBytes != 0 {
				t.Fatalf("preparation panic retained admission: %+v", census)
			}
		})
	}
}
