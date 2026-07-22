// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerationWrapRetiresExhaustedFreelistHeads(t *testing.T) {
	tests := map[string]func(*testing.T){"queued task": testQueuedTaskGenerationRetirement}
	for name, test := range tests {
		t.Run(name, test)
	}
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
		Work: frameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", nil)
		}),
	}

	ref, err := supervisor.Enqueue(TaskClassGenericFunction, plan)

	require.NoError(t, err)
	require.Equal(t, TaskRequestRef{Slot: 2, Generation: 8}, ref)
	require.NoError(t, supervisor.CancelPending(ref))
}
