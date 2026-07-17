// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestSchedulerTicksEachJobAndModuleOnce(t *testing.T) {
	reconciler := &schedulerTestReconciler{}
	scheduler, err := NewScheduler(reconciler)
	if err != nil {
		t.Fatal(err)
	}
	jobs := map[string]*schedulerTestJob{
		"first": {
			id: "first", module: "module-a",
		},
		"second": {
			id: "second", module: "module-a",
		},
		"third": {
			id: "third", module: "module-b",
		},
	}
	order := []string{"first", "second", "third"}
	for _, name := range order {
		job := jobs[name]
		if err := scheduler.Register(
			lifecycle.ResourceIdentity{ID: name, Generation: 1},
			job,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := scheduler.Tick(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	for _, name := range order {
		job := jobs[name]
		if job.ticks != 1 || job.clock != 7 {
			t.Fatalf(
				"job %s ticks=%d clock=%d want=1/7",
				name,
				job.ticks,
				job.clock,
			)
		}
	}
	if got := reconciler.snapshot(); !equalStrings(
		got,
		[]string{"module-a", "module-b"},
	) {
		t.Fatalf("module reconciliations=%v", got)
	}
	for name, job := range jobs {
		if err := scheduler.Unregister(
			lifecycle.ResourceIdentity{ID: name, Generation: 1},
			job,
		); err != nil {
			t.Fatal(err)
		}
	}
	if scheduler.Census() != 0 {
		t.Fatalf("scheduler census=%d want=0", scheduler.Census())
	}
	if err := scheduler.Tick(context.Background(), 8); err != nil {
		t.Fatal(err)
	}
	for name, job := range jobs {
		if job.ticks != 1 {
			t.Fatalf("unregistered job %s ticked again", name)
		}
	}
}

func TestSchedulerWarmTickDoesNotAllocate(t *testing.T) {
	scheduler, err := NewScheduler(testModuleReconciler{})
	if err != nil {
		t.Fatal(err)
	}
	job := &schedulerTestJob{id: "job", module: "module"}
	identity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	if err := scheduler.Register(identity, job); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := scheduler.Tick(ctx, 1); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(1_000, func() {
		if err := scheduler.Tick(ctx, 1); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warm scheduler tick allocations=%f want=0", allocations)
	}
	if err := scheduler.Unregister(identity, job); err != nil {
		t.Fatal(err)
	}
}

type schedulerTestJob struct {
	id     string
	module string
	ticks  int
	clock  int
}

func (job *schedulerTestJob) FullName() string   { return job.id }
func (job *schedulerTestJob) ModuleName() string { return job.module }
func (job *schedulerTestJob) Name() string       { return job.id }
func (*schedulerTestJob) IsRunning() bool        { return true }
func (job *schedulerTestJob) Collector() any     { return job }
func (*schedulerTestJob) StartManaged(chan<- struct{}) {
}
func (*schedulerTestJob) Stop()                               {}
func (*schedulerTestJob) Cleanup()                            {}
func (*schedulerTestJob) AutoDetection(context.Context) error { return nil }
func (*schedulerTestJob) AutoDetectionManaged(context.Context) error {
	return nil
}
func (*schedulerTestJob) CleanupRejected() {}
func (job *schedulerTestJob) Tick(clock int) {
	job.ticks++
	job.clock = clock
}

type schedulerTestReconciler struct {
	mu      sync.Mutex
	modules []string
}

func (reconciler *schedulerTestReconciler) ReconcileModule(
	_ context.Context,
	module string,
) error {
	reconciler.mu.Lock()
	reconciler.modules = append(reconciler.modules, module)
	reconciler.mu.Unlock()
	return nil
}

func (reconciler *schedulerTestReconciler) snapshot() []string {
	reconciler.mu.Lock()
	defer reconciler.mu.Unlock()
	return append([]string(nil), reconciler.modules...)
}
