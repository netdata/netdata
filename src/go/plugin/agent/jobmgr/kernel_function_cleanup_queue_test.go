// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type abortCleanupCatalog struct {
	functionCatalogPortStub
	cleanups []FunctionCleanupPlan
	err      error
}

func (acc *abortCleanupCatalog) AbortMutation(
	cleanups *[MaximumFunctionCleanupBatch]FunctionCleanupPlan,
) (int, error) {
	return copy(cleanups[:], acc.cleanups), acc.err
}

func TestFunctionCleanupQueuePreservesFIFOAndReleasesReferences(t *testing.T) {
	const population = 199

	var queue functionCleanupQueue
	for index := range population {
		plan := FunctionCleanupPlan{
			Ref: FunctionCleanupRef{
				Slot:       uint32(index + 1),
				Generation: 1,
			},
			Work: func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		}

		require.NoError(t, queue.push(plan))
	}
	require.EqualValues(t, population, queue.count)
	for index := range population {
		plan := queue.front()
		require.False(t, plan.Ref.Slot != uint32(index+1) || plan.Ref.Generation != 1)
		queue.pop()
	}
	require.Zero(t, queue.count)
	require.Zero(t, queue.head)
	require.Nil(t, queue.plans)

	plan := queue.front()
	require.False(t, plan.Ref.Valid() || plan.Work != nil)
}

func TestAbortMutationCleanupsPreservesOrderAndJoinsErrors(t *testing.T) {
	abortErr := errors.New("abort failed")
	work := func(context.Context) (lifecycle.TaskOutcome, error) {
		return lifecycle.NoValueOutcome(), nil
	}
	catalog := &abortCleanupCatalog{
		cleanups: []FunctionCleanupPlan{
			{Ref: FunctionCleanupRef{Slot: 1, Generation: 1}, Work: work},
			{Ref: FunctionCleanupRef{Slot: 2, Generation: 1}, Work: work},
			{Ref: FunctionCleanupRef{Slot: 3, Generation: 1}},
		},
		err: abortErr,
	}
	kernel := &CommandKernel{functionCatalog: catalog}

	err := kernel.abortMutationCleanups()
	require.ErrorIs(t, err, abortErr)
	assert.EqualError(t, err, "abort failed\njobmgr kernel: invalid Function cleanup plan")
	require.Equal(t, 2, kernel.functionCleanupBacklog.count)
	assert.EqualValues(t, 1, kernel.functionCleanupBacklog.front().Ref.Slot)
	kernel.functionCleanupBacklog.pop()
	assert.EqualValues(t, 2, kernel.functionCleanupBacklog.front().Ref.Slot)
}

func BenchmarkBFunctionCleanupQueuePushPop(b *testing.B) {
	const population = 256
	plans := make([]FunctionCleanupPlan, population)
	for index := range plans {
		plans[index] = FunctionCleanupPlan{
			Ref: FunctionCleanupRef{
				Slot:       uint32(index + 1),
				Generation: 1,
			},
			Work: func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		var queue functionCleanupQueue
		for _, plan := range plans {
			if err := queue.push(plan); err != nil {
				require.FailNow(b, "benchmark failed", err)
			}
		}
		for queue.count != 0 {
			queue.pop()
		}
	}
}
