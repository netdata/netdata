// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInheritedTaskRunCancelJoinRelease(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 7}
	entered := make(chan struct{})
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedPipelineProvider, func(ctx context.Context) error {
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
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedPipelineSupervisor, func(context.Context) error {
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
	if _, err := supervisor.StartInherited(context.Background(), owner, 0, func(context.Context) error { return nil }); err == nil {
		t.Fatal("invalid inherited role was accepted")
	}
}

func TestInheritedTaskMissedJoinRetainsRecord(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	releaseWork := make(chan struct{})
	finished := make(chan struct{})
	ref, err := supervisor.StartInherited(context.Background(), owner, InheritedPipelineProvider, func(context.Context) error {
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

func TestInheritedTaskCapacityIsDerivedFromGenerationLanes(t *testing.T) {
	supervisor := newResourceTaskSupervisor(t)
	refs := make([]InheritedTaskRef, 0, MaximumInheritedTasks)
	for index := 0; index < MaximumInheritedTasks; index++ {
		owner := ResourceIdentity{ID: "pipeline", Generation: uint64(index + 1)}
		ref, err := supervisor.StartInherited(context.Background(), owner, InheritedPipelineProvider, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		if err != nil {
			t.Fatalf("start %d: %v", index, err)
		}
		refs = append(refs, ref)
	}
	if _, err := supervisor.StartInherited(context.Background(), ResourceIdentity{ID: "overflow", Generation: 1}, InheritedPipelineProvider, func(context.Context) error { return nil }); err == nil {
		t.Fatal("inherited capacity admitted one extra child")
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
