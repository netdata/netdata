// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestFunctionCleanupQueuePreservesFIFOAndReleasesChunks(t *testing.T) {
	const population = 3*functionCleanupChunkCapacity + 7

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
	chunks := 0
	for chunk := queue.head; chunk != nil; chunk = chunk.next {
		chunks++
	}
	const wantChunks = 4
	require.EqualValues(t, wantChunks, chunks)

	firstChunk := queue.head
	for index := range population {
		plan := queue.front()
		require.False(t, plan.Ref.Slot != uint32(index+1) || plan.Ref.Generation != 1)
		queue.pop()
		if index == functionCleanupChunkCapacity-1 {
			require.Nil(t, firstChunk.next)
			for slot, plan := range firstChunk.plans {
				require.False(t, plan.Ref.Valid() || plan.Work != nil || plan.Runner != nil, "slot=%d", slot)
			}
		}
	}
	require.False(t, queue.count != 0 || queue.head != nil || queue.tail != nil)

	plan := queue.front()
	require.False(t, plan.Ref.Valid() || plan.Work != nil || plan.Runner != nil)
}

func BenchmarkBFunctionCleanupQueuePushPop(b *testing.B) {
	const population = 4 * functionCleanupChunkCapacity
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
