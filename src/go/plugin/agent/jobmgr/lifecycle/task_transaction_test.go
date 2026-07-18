// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
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
			events := []string{}
			result, err := NewSealedResult(200, "application/json", []byte(`{"ok":true}`))
			if err != nil {
				t.Fatal(err)
			}
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
			if err != nil {
				t.Fatal(err)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)

			first := <-supervisor.CompletionCh()
			if first.Ref != ref || first.Sequence != 1 ||
				first.Kind != TaskOutcomePreparedResourceTransaction ||
				first.Err != nil {
				t.Fatalf("initial completion=%#v", first)
			}
			if err := supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 2, Kind: TaskActionApplyResourceTransaction,
			}); err != nil {
				t.Fatal(err)
			}
			second := <-supervisor.CompletionCh()
			if second.Ref != ref || second.Sequence != 2 ||
				second.Kind != TaskOutcomeAppliedResourceTransaction ||
				second.Err != nil {
				t.Fatalf("apply completion=%#v", second)
			}
			disposition, current, err := supervisor.TakeAppliedResourceTransaction(
				ref,
				2,
				test.scope,
			)
			if err != nil {
				t.Fatal(err)
			}
			if disposition != test.disposition || current != test.resulting {
				t.Fatalf(
					"applied transaction disposition=%v current=%T, want %v %T",
					disposition,
					current,
					test.disposition,
					test.resulting,
				)
			}
			if _, err := supervisor.PreflightResult(ref, "tx", 1); err != nil {
				t.Fatal(err)
			}
			if err := supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 3, Kind: TaskActionEncodeWrite, UID: "tx", Expiry: 1,
			}); err != nil {
				t.Fatal(err)
			}
			if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
				t.Fatalf("encode acknowledgement=%#v", ack)
			}
			if err := supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 4, Kind: TaskActionCleanup,
			}); err != nil {
				t.Fatal(err)
			}
			if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
				t.Fatalf("cleanup acknowledgement=%#v", ack)
			}
			if err := supervisor.SendAction(TaskAction{
				Ref: ref, Sequence: 5, Kind: TaskActionTerminate,
			}); err != nil {
				t.Fatal(err)
			}
			if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
				t.Fatalf("termination acknowledgement=%#v", ack)
			}
			if err := supervisor.Release(ref); err != nil {
				t.Fatal(err)
			}
			if want := []string{"prepare", "apply", "cleanup"}; !reflect.DeepEqual(events, want) {
				t.Fatalf("events=%v, want %v", events, want)
			}
		})
	}
}

func TestTaskSupervisorDisposesPreparedTransactionAndRestoresCurrent(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	events := []string{}
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
	if err != nil {
		t.Fatal(err)
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, plan)
	<-supervisor.CompletionCh()

	if err := supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 2, Kind: TaskActionDispose,
	}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatalf("dispose acknowledgement=%#v", ack)
	}
	restored, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
	if err != nil {
		t.Fatal(err)
	}
	if restored != current {
		t.Fatalf("restored current=%T, want original resource", restored)
	}
	if err := supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: 3, Kind: TaskActionTerminate,
	}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatalf("termination acknowledgement=%#v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if want := []string{"prepare", "dispose"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v, want %v", events, want)
	}
}

func TestTaskSupervisorRejectedTransactionStartReturnsCurrent(t *testing.T) {
	tests := map[string]struct {
		scope         ResourceTransactionScope
		current       ReadyResource
		seededPermits int
		replacementAt int
	}{
		"replacement current": {
			scope: ResourceTransactionScope{
				ID:      "job",
				Current: ResourceIdentity{ID: "job", Generation: 1},
				Successor: ResourceIdentity{
					ID: "job", Generation: 2,
				},
			},
			current: &recordingReadyResource{
				identity: ResourceIdentity{ID: "job", Generation: 1},
				events:   new([]string),
			},
			seededPermits: 1 + MaximumReplacementOverlaps,
			replacementAt: 1,
		},
		"installation has no current": {
			scope: ResourceTransactionScope{
				ID:        "job",
				Successor: ResourceIdentity{ID: "job", Generation: 1},
			},
			seededPermits: 1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			admission := NewAdmissionLedger()
			supervisor := newResourceTaskSupervisor(t)
			longLived := make([]LongLivedPermit, 0, test.seededPermits)
			var grants [TransientTaskSlots]AdmissionGrant
			for index := 0; index < test.seededPermits; index++ {
				request := admission.RequestOrdinary(
					1,
					AdmissionLaneRef{
						Slot:       uint32(index + 1),
						Generation: 1,
					},
					2,
				)
				if request.Rejected != nil {
					t.Fatal(request.Rejected)
				}
				count, _, err := admission.TakeGrants(1, &grants)
				if err != nil || count != 1 {
					t.Fatalf(
						"long-lived grant %d count=%d err=%v",
						index,
						count,
						err,
					)
				}
				permitPlan, err := NewSecretStoreLongLivedPlan(1)
				if err != nil {
					t.Fatal(err)
				}
				if index >= test.replacementAt && test.replacementAt != 0 {
					permitPlan.replacementOverlap = true
				}
				permit, err := supervisor.IssueLongLivedPermit(
					admission,
					request.Ref,
					ResourceIdentity{
						ID:         fmt.Sprintf("seed-%03d", index),
						Generation: 1,
					},
					permitPlan,
				)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := admission.ReleaseOrdinary(
					request.Ref,
				); err != nil {
					t.Fatal(err)
				}
				longLived = append(longLived, permit)
			}

			transactionAdmission := admission.RequestOrdinary(
				1,
				AdmissionLaneRef{
					Slot:       1,
					Generation: 2,
				},
				2,
			)
			if transactionAdmission.Rejected != nil {
				t.Fatal(transactionAdmission.Rejected)
			}
			count, _, err := admission.TakeGrants(1, &grants)
			if err != nil || count != 1 {
				t.Fatalf(
					"transaction grant count=%d err=%v",
					count,
					err,
				)
			}
			permitPlan, err := NewSecretStoreLongLivedPlan(1)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := NewResourceTransactionPermitTaskPlan(
				SourceJobManager,
				time.Time{},
				TransactionTaskPhases,
				admission,
				transactionAdmission.Ref,
				test.current,
				test.scope,
				permitPlan,
				func(
					context.Context,
					ReadyResource,
					ResourceTransactionScope,
					LongLivedPermit,
				) (PreparedResourceTransaction, error) {
					t.Fatal("capacity-rejected transaction ran")
					return nil, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			requestRef, err := supervisor.Enqueue(
				TaskClassFrameworkControl,
				plan,
			)
			if err != nil {
				t.Fatal(err)
			}
			var started [TransientTaskSlots]TaskStart
			count, _, err = supervisor.Dispatch(
				context.Background(),
				1,
				&started,
			)
			if err != nil ||
				count != 1 ||
				started[0].Request != requestRef ||
				!errors.Is(
					started[0].Err,
					ErrLongLivedRecordCapacity,
				) {
				t.Fatalf(
					"rejected start count=%d start=%#v err=%v",
					count,
					started[0],
					err,
				)
			}
			if test.current != nil {
				current, ok := started[0].Outcome.ReadyResource()
				identity, identityOK :=
					started[0].Outcome.ResourceIdentity()
				if !ok ||
					!identityOK ||
					current != test.current ||
					identity != test.scope.Current {
					t.Fatalf(
						"returned current=%T identity=%#v",
						current,
						identity,
					)
				}
			} else if started[0].Outcome.Kind() != TaskOutcomeNone {
				t.Fatalf(
					"graph-only rejected outcome=%v",
					started[0].Outcome.Kind(),
				)
			}
			if supervisor.Active() != 0 || supervisor.Pending() != 0 {
				t.Fatalf(
					"rejected start retained tasks: active=%d pending=%d",
					supervisor.Active(),
					supervisor.Pending(),
				)
			}
			if _, err := admission.ReleaseOrdinary(
				transactionAdmission.Ref,
			); err != nil {
				t.Fatal(err)
			}
			for _, permit := range longLived {
				if err := permit.AbortUnused(); err != nil {
					t.Fatal(err)
				}
			}
			if census := admission.Census(); census.ActiveRecords != 0 ||
				census.OrdinaryBytes != 0 ||
				census.LongLivedBytes != 0 {
				t.Fatalf("released census=%+v", census)
			}
		})
	}
}

type recordingPreparedResourceTransaction struct {
	scope   ResourceTransactionScope
	current ReadyResource
	applied AppliedResourceTransaction
	events  *[]string
}

func (transaction *recordingPreparedResourceTransaction) Scope() ResourceTransactionScope {
	return transaction.scope
}

func (transaction *recordingPreparedResourceTransaction) Apply(
	context.Context,
) (AppliedResourceTransaction, error) {
	*transaction.events = append(*transaction.events, "apply")
	return transaction.applied, nil
}

func (transaction *recordingPreparedResourceTransaction) Dispose(
	context.Context,
) (ReadyResource, error) {
	*transaction.events = append(*transaction.events, "dispose")
	return transaction.current, nil
}
