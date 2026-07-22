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
	err error
}

func (acc *abortCleanupCatalog) AbortMutation(FunctionCatalogMutation) error {
	return acc.err
}

func TestFunctionCleanupQueuePreservesFIFOAndReleasesReferences(t *testing.T) {
	const population = 199

	var queue functionCleanupQueue
	for index := range population {
		plan := FunctionCleanupPlan{
			Ref: FunctionCleanupRef(index + 1),
			Work: func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		}

		require.NoError(t, queue.push(plan))
	}
	require.EqualValues(t, population, queue.count)
	for index := range population {
		plan := queue.front()
		require.Equal(t, FunctionCleanupRef(index+1), plan.Ref)
		queue.pop()
	}
	require.Zero(t, queue.count)
	require.Zero(t, queue.head)
	require.Nil(t, queue.plans)

	plan := queue.front()
	require.False(t, plan.Ref.Valid() || plan.Work != nil)
}

func TestFunctionCleanupQueueStorageTracksLiveBacklog(t *testing.T) {
	const population = 10_000

	plan := func(slot uint32) FunctionCleanupPlan {
		return FunctionCleanupPlan{
			Ref: FunctionCleanupRef(slot),
			Work: func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		}
	}
	var queue functionCleanupQueue
	for slot := uint32(1); slot <= population; slot++ {
		require.NoError(t, queue.push(plan(slot)))
	}
	for queue.count > 1 {
		queue.pop()
	}
	require.NoError(t, queue.push(plan(population+1)))
	queue.pop()

	require.Equal(t, 1, queue.count)
	assert.LessOrEqual(t, len(queue.plans), 2*queue.count)
	assert.LessOrEqual(t, cap(queue.plans), 2*queue.count)
	assert.Equal(t, FunctionCleanupRef(population+1), queue.front().Ref)
}

func TestAbortFunctionMutationReturnsCatalogError(t *testing.T) {
	abortErr := errors.New("abort failed")
	catalog := &abortCleanupCatalog{err: abortErr}
	kernel := &CommandKernel{functionCatalog: catalog}

	err := kernel.abortFunctionMutation(nil)
	require.ErrorIs(t, err, abortErr)
	assert.EqualError(t, err, "abort failed")
}

func BenchmarkBFunctionCleanupQueuePushPop(b *testing.B) {
	const population = 256
	plans := make([]FunctionCleanupPlan, population)
	for index := range plans {
		plans[index] = FunctionCleanupPlan{
			Ref: FunctionCleanupRef(index + 1),
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
