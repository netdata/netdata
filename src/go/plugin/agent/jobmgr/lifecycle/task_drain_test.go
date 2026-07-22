// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDrainDependentTasksDoNotConsumeGlobalExecutionCapacity(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	drainRequests := make(map[TaskRequestRef]struct{}, TaskStartServiceQuantum+1)
	for index := range TaskStartServiceQuantum + 1 {
		resource := &recordingReadyResource{
			identity: ResourceIdentity{ID: "job", Generation: uint64(index + 1)},
			events:   &events,
		}
		plan := readyTaskPlan(t, SourceJobManager, time.Time{}, resource)
		request, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
		require.NoError(t, err)
		drainRequests[request] = struct{}{}
	}
	progressRequest, err := supervisor.Enqueue(
		TaskClassGenericFunction,
		TaskPlan{
			Source: SourceFunction,
			Work: func(context.Context) (TaskOutcome, error) {
				return NoValueOutcome(), nil
			},
		},
	)
	require.NoError(t, err)

	started := make([]TaskStart, 0, len(drainRequests)+1)
	for {
		var starts [TaskStartServiceQuantum]TaskStart
		count, more, err := supervisor.Dispatch(context.Background(), TaskStartServiceQuantum, &starts)
		require.NoError(t, err)
		require.False(t, count > TaskStartServiceQuantum)
		started = append(started, starts[:count]...)
		if !more {
			break
		}
	}
	require.EqualValues(t, len(drainRequests)+1, len(started))
	require.EqualValues(t, len(started), supervisor.Active())

	byTask := make(map[TaskRef]TaskRequestRef, len(started))
	for _, start := range started {
		byTask[start.Task] = start.Request
	}
	for range started {
		completion := <-supervisor.CompletionCh()
		request := byTask[completion.Ref]
		if request == progressRequest {
			terminateAndReleaseTask(t, supervisor, completion.Ref, 2)
			continue
		}

		_, ok := drainRequests[request]
		require.True(t, ok)

		require.NoError(t, supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose,
		}),
		)

		ack := <-supervisor.AcknowledgementCh()
		require.Nil(t, ack.Err)

		terminateAndReleaseTask(t, supervisor, completion.Ref, 3)
	}
	require.False(t, supervisor.Active() != 0 || supervisor.Pending() != 0)
}

func terminateAndReleaseTask(t *testing.T, supervisor *TaskSupervisor, ref TaskRef, sequence uint8) {
	t.Helper()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: sequence, Kind: TaskActionTerminate}))

	ack := <-supervisor.AcknowledgementCh()
	require.Nil(t, ack.Err)

	require.NoError(t, supervisor.Release(ref))
}
