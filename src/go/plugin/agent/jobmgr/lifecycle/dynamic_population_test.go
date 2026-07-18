// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
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
		if result.Rejected != nil {
			t.Fatalf("admit record %d: %v", index, result.Rejected)
		}
		refs = append(refs, result.Ref)
	}
	for _, ref := range refs {
		if err := ledger.CancelWaiting(ref); err != nil {
			t.Fatal(err)
		}
	}
}

func testAdmissionLaneGrowth(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	refs := make([]AdmissionRef, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		result := ledger.RequestOrdinary(1, lane, 1)
		if result.Rejected != nil {
			t.Fatalf("admit lane record %d: %v", index, result.Rejected)
		}
		refs = append(refs, result.Ref)
	}
	for _, ref := range refs {
		if err := ledger.CancelWaiting(ref); err != nil {
			t.Fatal(err)
		}
	}
}

func testTaskRequestGrowth(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
	plan := TaskPlan{
		Source: SourceJobManager,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
	}
	refs := make([]TaskRequestRef, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		ref, enqueueErr := supervisor.Enqueue(plan)
		if enqueueErr != nil {
			t.Fatalf("enqueue request %d: %v", index, enqueueErr)
		}
		refs = append(refs, ref)
	}
	for _, ref := range refs {
		if err := supervisor.CancelPending(ref); err != nil {
			t.Fatal(err)
		}
	}
}

func testLongLivedJobGrowth(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewJobLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
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
		if issueErr != nil {
			t.Fatalf("issue job permit %d: %v", index, issueErr)
		}
		if _, releaseErr := admission.ReleaseOrdinary(ref); releaseErr != nil {
			t.Fatal(releaseErr)
		}
		permits = append(permits, permit)
	}
	for _, permit := range permits {
		if err := permit.AbortUnused(); err != nil {
			t.Fatal(err)
		}
	}
}

func testUIDGrowth(t *testing.T) {
	ledger := NewUIDLedger()
	const population = formerFixedUIDPopulation + 1
	now := time.Unix(1, 0)
	for index := 0; index < population; index++ {
		if err := ledger.Admit(
			fmt.Sprintf("uid-%05d", index),
			now,
		); err != nil {
			t.Fatalf("admit UID %d: %v", index, err)
		}
	}
	for index := 0; index < population; index++ {
		if err := ledger.Complete(
			fmt.Sprintf("uid-%05d", index),
			false,
			now,
		); err != nil {
			t.Fatalf("complete UID %d: %v", index, err)
		}
	}
}
