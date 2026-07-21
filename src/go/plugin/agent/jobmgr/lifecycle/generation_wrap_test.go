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
		"admission record": testAdmissionRecordGenerationRetirement,
		"queued task":      testQueuedTaskGenerationRetirement,
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
