// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"testing"
	"time"
)

func TestDrainDependentTasksReserveOneProgressSlot(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	var events []string
	var starts [TransientTaskSlots]TaskStart
	var drainRequests [TransientTaskSlots]TaskRequestRef
	for index := range drainRequests {
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
		drainRequests[index] = request
	}
	count, more, err := supervisor.Dispatch(context.Background(), len(starts), &starts)
	if err != nil {
		t.Fatal(err)
	}
	if count != maximumConcurrentDrainDependentTasks || !more {
		t.Fatalf("started=%d more=%v", count, more)
	}
	activeDrains := append([]TaskStart(nil), starts[:count]...)
	for range count {
		<-supervisor.CompletionCh()
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
	starts = [TransientTaskSlots]TaskStart{}
	count, _, err = supervisor.Dispatch(context.Background(), 1, &starts)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || starts[0].Request != progressRequest {
		t.Fatalf("progress start=%+v", starts[0])
	}
	<-supervisor.CompletionCh()
	terminateAndReleaseTask(t, supervisor, starts[0].Task, 2)

	for _, started := range activeDrains {
		if err := supervisor.SendAction(TaskAction{
			Ref: started.Task, Sequence: 2, Kind: TaskActionDispose,
		}); err != nil {
			t.Fatal(err)
		}
		if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
			t.Fatal(ack.Err)
		}
		terminateAndReleaseTask(t, supervisor, started.Task, 3)
	}

	starts = [TransientTaskSlots]TaskStart{}
	count, more, err = supervisor.Dispatch(context.Background(), 1, &starts)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || more || starts[0].Request != drainRequests[len(drainRequests)-1] {
		t.Fatalf("final drain start=%+v count=%d more=%v", starts[0], count, more)
	}
	<-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{
		Ref: starts[0].Task, Sequence: 2, Kind: TaskActionDispose,
	}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	terminateAndReleaseTask(t, supervisor, starts[0].Task, 3)
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
