// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInheritedTaskRunCancelJoinRelease(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 7}
	entered := make(chan struct{})
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV1Runtime, func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if supervisor.Active() != 0 || supervisor.InheritedActive() != 1 {
		t.Fatalf("transient=%d inherited=%d", supervisor.Active(), supervisor.InheritedActive())
	}
	if err := supervisor.CancelInherited(ref, owner); err != nil {
		t.Fatal(err)
	}
	if joined, err := supervisor.JoinInherited(context.Background(), ref, owner); err != nil || !joined {
		t.Fatal(err)
	}
	if err := supervisor.ReleaseInherited(ref, owner); err != nil {
		t.Fatal(err)
	}
	if supervisor.InheritedActive() != 0 {
		t.Fatalf("inherited=%d", supervisor.InheritedActive())
	}
	if err := supervisor.ReleaseInherited(ref, owner); err == nil {
		t.Fatal("stale inherited reference released twice")
	}
}

func TestInheritedTaskOwnerRoleAndPanicAreContained(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	observer := &recordingRuntimeObserver{}
	if err := supervisor.BindRuntimeObserver(observer); err != nil {
		t.Fatal(err)
	}
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV2Runner, func(context.Context) error {
		panic("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongOwner := ResourceIdentity{ID: owner.ID, Generation: owner.Generation + 1}
	if err := supervisor.CancelInherited(ref, wrongOwner); err == nil {
		t.Fatal("wrong owner canceled inherited child")
	}
	if err := supervisor.CancelInherited(ref, owner); err != nil {
		t.Fatal(err)
	}
	if joined, err := supervisor.JoinInherited(context.Background(), ref, owner); !joined || !errors.Is(err, ErrTaskPanic) {
		t.Fatalf("joined=%v join error=%v", joined, err)
	}
	if err := supervisor.ReleaseInherited(ref, owner); err != nil {
		t.Fatal(err)
	}
	if got := observer.counter(RuntimeCounterTaskPanics); got != 1 {
		t.Fatalf("task panics=%d want=1", got)
	}
	if _, err := supervisor.StartInherited(context.Background(), owner, 0, func(context.Context) error { return nil }); err == nil {
		t.Fatal("invalid inherited role was accepted")
	}
}

type recordingRuntimeObserver struct {
	mu       sync.Mutex
	counters map[RuntimeCounter]uint64
}

func (*recordingRuntimeObserver) SetRuntimeGauge(RuntimeGauge, int) {}
func (*recordingRuntimeObserver) AddRuntimeGauge(RuntimeGauge, int) {}
func (*recordingRuntimeObserver) SetRuntimeTimestamp(RuntimeTimestamp, time.Time) {
}

func (observer *recordingRuntimeObserver) AddRuntimeCounter(
	kind RuntimeCounter,
	delta uint64,
) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.counters == nil {
		observer.counters = make(map[RuntimeCounter]uint64)
	}
	observer.counters[kind] += delta
}

func (observer *recordingRuntimeObserver) counter(
	kind RuntimeCounter,
) uint64 {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.counters[kind]
}

func TestInheritedTaskMissedJoinRetainsRecord(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	releaseWork := make(chan struct{})
	finished := make(chan struct{})
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV1Runtime, func(context.Context) error {
		defer close(finished)
		<-releaseWork
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.CancelInherited(ref, owner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if joined, err := supervisor.JoinInherited(ctx, ref, owner); joined || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("joined=%v join error=%v", joined, err)
	}
	if err := supervisor.ReleaseInherited(ref, owner); err == nil {
		t.Fatal("missed join released inherited record")
	}
	if supervisor.InheritedActive() != 1 {
		t.Fatalf("inherited=%d", supervisor.InheritedActive())
	}
	close(releaseWork)
	<-finished
}

func TestInheritedTasksGrowBeyondFormerDerivedLimit(t *testing.T) {
	const population = 2*formerFixedPopulation + 1
	supervisor := newResourceTaskSupervisor(t)
	refs := make([]InheritedTaskRef, 0, population)
	for index := 0; index < population; index++ {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}
		ref, err := supervisor.StartInherited(context.Background(), owner, InheritedV1Runtime, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		if err != nil {
			t.Fatalf("start %d: %v", index, err)
		}
		refs = append(refs, ref)
	}
	for index, ref := range refs {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}
		if err := supervisor.CancelInherited(ref, owner); err != nil {
			t.Fatal(err)
		}
	}
	for index, ref := range refs {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}
		if joined, err := supervisor.JoinInherited(context.Background(), ref, owner); err != nil || !joined {
			t.Fatal(err)
		}
		if err := supervisor.ReleaseInherited(ref, owner); err != nil {
			t.Fatal(err)
		}
	}
	if supervisor.InheritedActive() != 0 {
		t.Fatalf("inherited=%d", supervisor.InheritedActive())
	}
}

func TestInheritedPipelineTasksRequirePermit(t *testing.T) {
	tests := map[string]InheritedTaskRole{
		"provider":   InheritedPipelineProvider,
		"supervisor": InheritedPipelineSupervisor,
	}
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	for name, role := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := supervisor.StartInherited(
				context.Background(),
				owner,
				role,
				func(context.Context) error { return nil },
			); err == nil {
				t.Fatal("pipeline task started without a permit")
			}
		})
	}
}
