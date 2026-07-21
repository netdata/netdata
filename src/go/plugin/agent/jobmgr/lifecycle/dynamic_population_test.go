// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const formerFixedPopulation = 256
const formerFixedUIDPopulation = 16_384

func TestAdmissionPopulationGrowsBeyondFormerLimits(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T)
	}{
		"admission records": {
			run: testAdmissionRecordGrowth,
		},
		"same admission lane": {
			run: testAdmissionLaneGrowth,
		},
	}
	for name, test := range tests {
		t.Run(name, test.run)
	}
}

func TestPendingTaskRequestPopulationGrowsBeyondFormerLimit(t *testing.T) {
	testTaskRequestGrowth(t)
}

func TestLongLivedJobPermitPopulationGrowsBeyondFormerLimit(t *testing.T) {
	testLongLivedJobGrowth(t)
}

func TestActiveFunctionUIDPopulationGrowsBeyondFormerLimit(t *testing.T) {
	testUIDGrowth(t)
}

func testAdmissionRecordGrowth(t *testing.T) {
	ledger := NewAdmissionLedger()
	refs := make([]AdmissionRef, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		result := ledger.RequestOrdinary(
			1,
			AdmissionLaneRef{
				Slot:       uint32(index + 1),
				Generation: 1,
			},
			1,
		)
		require.Nil(t, result.Rejected)
		refs = append(refs, result.Ref)
	}
	for _, ref := range refs {
		require.NoError(t, ledger.CancelWaiting(ref))
	}
}

func testAdmissionLaneGrowth(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	refs := make([]AdmissionRef, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		result := ledger.RequestOrdinary(1, lane, 1)
		require.Nil(t, result.Rejected)
		refs = append(refs, result.Ref)
	}
	for _, ref := range refs {
		require.NoError(t, ledger.CancelWaiting(ref))
	}
}

func testTaskRequestGrowth(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	plan := TaskPlan{
		Source: SourceJobManager,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
	}
	refs := make([]TaskRequestRef, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		ref, enqueueErr := supervisor.Enqueue(TaskClassFrameworkControl, plan)
		require.NoError(t, enqueueErr)
		refs = append(refs, ref)
	}
	for _, ref := range refs {
		require.NoError(t, supervisor.CancelPending(ref))
	}
}

func testLongLivedJobGrowth(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan := NewJobLongLivedPlan()
	permits := make([]LongLivedPermit, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		ref := grantLongLivedTestAdmission(t, admission, 2)
		permit, issueErr := supervisor.IssueLongLivedPermit(
			admission,
			ref,
			ResourceIdentity{
				ID:         fmt.Sprintf("job-%03d", index),
				Generation: 1,
			},
			plan,
		)
		require.NoError(t, issueErr)

		_, releaseErr := admission.ReleaseOrdinary(ref)
		require.NoError(t, releaseErr)

		permits = append(permits, permit)
	}
	for _, permit := range permits {
		require.NoError(t, permit.AbortUnused())
	}
}

func testUIDGrowth(t *testing.T) {
	ledger := NewUIDLedger()
	const population = formerFixedUIDPopulation + 1
	now := time.Unix(1, 0)
	for index := range population {
		require.NoError(t, ledger.Admit(fmt.Sprintf("uid-%05d", index), now))
	}
	for index := range population {
		require.NoError(t, ledger.Complete(fmt.Sprintf("uid-%05d", index), false, now))
	}
}
