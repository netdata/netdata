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
		plan, err := NewFunctionCleanupPlan(
			FunctionCleanupRef(index+1),
			func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		)
		require.NoError(t, err)

		queue.push(plan)
	}
	require.EqualValues(t, population, queue.count)
	for index := range population {
		plan := queue.front()
		require.Equal(t, FunctionCleanupRef(index+1), plan.Ref())
		queue.pop()
	}
	require.Zero(t, queue.count)
	require.Nil(t, queue.head)
	require.Nil(t, queue.tail)

	plan := queue.front()
	require.False(t, plan.Valid() || plan.Work() != nil)
}

func TestFunctionCleanupPlanRejectsMalformedOwnership(t *testing.T) {
	work := func(context.Context) (lifecycle.TaskOutcome, error) {
		return lifecycle.NoValueOutcome(), nil
	}
	tests := map[string]struct {
		ref  FunctionCleanupRef
		work lifecycle.TaskWork
	}{
		"missing reference": {work: work},
		"missing work":      {ref: 1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plan, err := NewFunctionCleanupPlan(test.ref, test.work)
			require.Error(t, err)
			assert.False(t, plan.Valid())
			assert.Nil(t, plan.Work())
		})
	}
}

func TestFunctionCleanupQueueReleasesConsumedChunks(t *testing.T) {
	const population = 10_000

	plan := func(slot uint32) FunctionCleanupPlan {
		cleanup, err := NewFunctionCleanupPlan(
			FunctionCleanupRef(slot),
			func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		)
		require.NoError(t, err)
		return cleanup
	}
	var queue functionCleanupQueue
	for slot := uint32(1); slot <= population; slot++ {
		queue.push(plan(slot))
	}
	for queue.count > 1 {
		queue.pop()
	}
	require.Equal(t, 1, queue.count)
	require.NotNil(t, queue.head)
	require.Same(t, queue.head, queue.tail)
	require.Nil(t, queue.head.next)
	require.Equal(t, FunctionCleanupRef(population), queue.front().Ref())

	queue.push(plan(population + 1))
	queue.pop()
	require.Equal(t, 1, queue.count)
	assert.Equal(t, FunctionCleanupRef(population+1), queue.front().Ref())
	queue.pop()
	require.Zero(t, queue.count)
	require.Nil(t, queue.head)
	require.Nil(t, queue.tail)
}

func TestAbortFunctionMutationReturnsCatalogError(t *testing.T) {
	abortErr := errors.New("abort failed")
	catalog := &abortCleanupCatalog{
		err: abortErr,
	}
	kernel := &CommandKernel{
		functionCatalog: catalog,
	}

	err := kernel.abortFunctionMutation(nil)
	require.ErrorIs(t, err, abortErr)
	assert.EqualError(t, err, "abort failed")
}

func BenchmarkBFunctionCleanupQueuePushPop(b *testing.B) {
	const population = 256
	plans := make([]FunctionCleanupPlan, population)
	for index := range plans {
		plan, err := NewFunctionCleanupPlan(
			FunctionCleanupRef(index+1),
			func(context.Context) (lifecycle.TaskOutcome, error) {
				return lifecycle.NoValueOutcome(), nil
			},
		)
		require.NoError(b, err)
		plans[index] = plan
	}
	b.ReportAllocs()
	for b.Loop() {
		var queue functionCleanupQueue
		for _, plan := range plans {
			queue.push(plan)
		}
		for queue.count != 0 {
			queue.pop()
		}
	}
}
