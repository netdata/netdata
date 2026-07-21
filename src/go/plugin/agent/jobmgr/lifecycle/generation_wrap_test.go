// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerationWrapRetiresExhaustedFreelistHeads(t *testing.T) {
	tests := map[string]func(*testing.T){
		"admission record":  testAdmissionRecordGenerationRetirement,
		"inherited task":    testInheritedTaskGenerationRetirement,
		"long-lived permit": testLongLivedGenerationRetirement,
		"queued task":       testQueuedTaskGenerationRetirement,
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func testAdmissionRecordGenerationRetirement(t *testing.T) {
	ledger := NewAdmissionLedger()
	ledger.records = append(
		ledger.records,
		admissionRecord{generation: math.MaxUint32, next: 3},
		admissionRecord{generation: 7},
	)
	ledger.freeRecordHead = 2
	ledger.freeRecords = 2
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}

	ref, err := ledger.allocateRecord(
		1,
		lane,
		1,
		admissionOrdinaryWaiting,
	)

	require.NoError(t, err)
	require.Equal(t, AdmissionRef{Slot: 3, Generation: 8}, ref)
	ledger.freeRecord(ref.Slot)
	require.True(t, ledger.allOperationRecordsFree())
}

func testInheritedTaskGenerationRetirement(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	supervisor.inherited.slots = []*inheritedTaskSlot{
		{generation: math.MaxUint32, freeNext: 2},
		{generation: 7},
	}
	supervisor.inherited.freeHead = 1
	owner := ResourceIdentity{ID: "runtime", Generation: 1}

	ref, err := supervisor.StartInherited(
		t.Context(),
		owner,
		InheritedV1Runtime,
		func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, InheritedTaskRef{Slot: 1, Generation: 8}, ref)
	require.NoError(t, supervisor.CancelInherited(ref, owner))
	joined, err := supervisor.JoinInherited(t.Context(), ref, owner)
	require.NoError(t, err)
	require.True(t, joined)
	require.NoError(t, supervisor.ReleaseInherited(ref, owner))
}

func testLongLivedGenerationRetirement(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	supervisor.longLived.slots = []*longLivedSlot{
		{generation: math.MaxUint32, freeNext: 2},
		{generation: 7},
	}
	supervisor.longLived.freeHead = 1
	plan := NewJobLongLivedPlan()
	admissionRef := grantLongLivedTestAdmission(t, admission, 1)

	permit, err := supervisor.IssueLongLivedPermit(
		admission,
		admissionRef,
		ResourceIdentity{ID: "job", Generation: 1},
		plan,
	)

	require.NoError(t, err)
	require.Equal(t, uint32(1), permit.ref.Slot)
	require.Equal(t, uint32(8), permit.ref.Generation)
	require.NoError(t, permit.AbortUnused())
	_, err = admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, err)
}

func testQueuedTaskGenerationRetirement(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	supervisor.requests = []*taskRequest{
		nil,
		{slot: 1, generation: math.MaxUint32, freeNext: 2},
		{slot: 2, generation: 7},
	}
	supervisor.freeRequest = 1
	plan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", nil)
		}),
	}

	ref, err := supervisor.Enqueue(TaskClassGenericFunction, plan)

	require.NoError(t, err)
	require.Equal(t, TaskRequestRef{Slot: 2, Generation: 8}, ref)
	require.NoError(t, supervisor.CancelPending(ref))
}
