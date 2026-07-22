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

func TestPendingTaskRequestPopulationGrowsBeyondFormerLimit(t *testing.T) {
	testTaskRequestGrowth(t)
}

func TestLongLivedJobPermitPopulationGrowsBeyondFormerLimit(t *testing.T) {
	testLongLivedJobGrowth(t)
}

func TestActiveFunctionUIDPopulationGrowsBeyondFormerLimit(t *testing.T) {
	testUIDGrowth(t)
}

func testTaskRequestGrowth(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	plan := TaskPlan{
		Source: SourceJobManager,
		Work: frameTaskWork(func(context.Context) (SealedResult, error) {
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
	supervisor := newLongLivedTestSupervisor(t)
	plan := NewJobLongLivedPlan()
	permits := make([]LongLivedPermit, 0, formerFixedPopulation+1)
	for index := 0; index <= formerFixedPopulation; index++ {
		permit, issueErr := supervisor.IssueLongLivedPermit(
			ResourceIdentity{ID: fmt.Sprintf("job-%03d", index), Generation: 1},
			plan,
		)
		require.NoError(t, issueErr)
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
