// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"testing"
	"time"
)

func TestDrainDependentTasksDoNotConsumeGlobalExecutionCapacity(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	drainRequests := make(map[TaskRequestRef]struct{}, TaskStartServiceQuantum+1)
	for index := 0; index < TaskStartServiceQuantum+1; index++ {
		resource := &recordingReadyResource{
			identity: ResourceIdentity{
				ID: "job", Generation: uint64(index + 1),
			},
			events: &events,
		}
		plan := readyTaskPlan(t, SourceJobManager, time.Time{}, resource)
		request, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
		if err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}

	started := make([]TaskStart, 0, len(drainRequests)+1)
	for {
		var starts [TaskStartServiceQuantum]TaskStart
		count, more, err := supervisor.Dispatch(
			context.Background(),
			TaskStartServiceQuantum,
			&starts,
		)
		if err != nil {
			t.Fatal(err)
		}
		if count > TaskStartServiceQuantum {
			t.Fatalf("one dispatch started %d tasks", count)
		}
		started = append(started, starts[:count]...)
		if !more {
			break
		}
	}
	if len(started) != len(drainRequests)+1 {
		t.Fatalf("started=%d want=%d", len(started), len(drainRequests)+1)
	}
	if supervisor.Active() != len(started) {
		t.Fatalf("active=%d want=%d", supervisor.Active(), len(started))
	}

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
		if _, ok := drainRequests[request]; !ok {
			t.Fatalf("completion for unknown request=%+v", request)
		}
		if err := supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose,
		}); err != nil {
			t.Fatal(err)
		}
		if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
			t.Fatal(ack.Err)
		}
		terminateAndReleaseTask(t, supervisor, completion.Ref, 3)
	}
	if supervisor.Active() != 0 || supervisor.Pending() != 0 {
		t.Fatalf(
			"terminal task census active=%d pending=%d",
			supervisor.Active(),
			supervisor.Pending(),
		)
	}
}

func terminateAndReleaseTask(
	t *testing.T,
	supervisor *TaskSupervisor,
	ref TaskRef,
	sequence uint8,
) {
	t.Helper()
	if err := supervisor.SendAction(TaskAction{
		Ref: ref, Sequence: sequence, Kind: TaskActionTerminate,
	}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}
