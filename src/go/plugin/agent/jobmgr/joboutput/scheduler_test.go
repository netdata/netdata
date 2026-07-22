// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestSchedulerTicksEachJobAndModuleOnce(t *testing.T) {
	reconciler := &schedulerTestReconciler{}
	scheduler, err := NewScheduler(reconciler)
	require.NoError(t, err)
	jobs := map[string]*schedulerTestJob{
		"first":  {id: "first", module: "module-a"},
		"second": {id: "second", module: "module-a"},
		"third":  {id: "third", module: "module-b"},
	}
	order := []string{"first", "second", "third"}
	for _, name := range order {
		job := jobs[name]

		require.NoError(t, scheduler.Register(lifecycle.ResourceIdentity{
			ID:         name,
			Generation: 1,
		}, job))
	}

	require.NoError(t, scheduler.Tick(context.Background(), 7))

	for _, name := range order {
		job := jobs[name]
		require.False(t, job.ticks != 1 || job.clock != 7)
	}

	require.ElementsMatch(t, []string{"module-a", "module-b"}, reconciler.snapshot())

	for name, job := range jobs {
		require.NoError(t, scheduler.Unregister(lifecycle.ResourceIdentity{
			ID:         name,
			Generation: 1,
		}, job))
	}
	require.NoError(t, scheduler.Tick(context.Background(), 8))

	for name, job := range jobs {
		require.EqualValues(t, 1, job.ticks, "job=%s", name)
	}
}

func TestSchedulerGrowsBeyondFormerActiveJobLimit(t *testing.T) {
	tests := map[string]struct {
		population int
	}{
		"former limit":          {population: 256},
		"former limit plus one": {population: 257},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			scheduler, err := NewScheduler(testModuleReconciler{})
			require.NoError(t, err)
			jobs := make([]*schedulerTestJob, 0, test.population)
			for index := 0; index < test.population; index++ {
				id := fmt.Sprintf("dynamic-job-%03d", index)
				job := &schedulerTestJob{
					id:     id,
					module: "module",
				}

				require.NoError(t, scheduler.Register(lifecycle.ResourceIdentity{
					ID:         id,
					Generation: 1,
				}, job))

				jobs = append(jobs, job)
			}
			require.NoError(t, scheduler.Tick(context.Background(), 1))
			for _, job := range jobs {
				require.EqualValues(t, 1, job.ticks, "job=%s", job.id)
			}
			for _, job := range jobs {
				require.NoError(t, scheduler.Unregister(lifecycle.ResourceIdentity{
					ID:         job.id,
					Generation: 1,
				}, job))
			}
			require.NoError(t, scheduler.Tick(context.Background(), 2))
			for _, job := range jobs {
				require.EqualValues(t, 1, job.ticks, "job=%s", job.id)
			}
		})
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
func (*schedulerTestJob) Stop()    {}
func (*schedulerTestJob) Cleanup() {}
func (*schedulerTestJob) AutoDetectionManaged(context.Context) error {
	return nil
}
func (*schedulerTestJob) AutoDetectionEvery() int { return 0 }
func (*schedulerTestJob) RetryAutoDetection() bool {
	return false
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

func (str *schedulerTestReconciler) ReconcileModule(_ context.Context, module string) error {
	str.mu.Lock()
	str.modules = append(str.modules, module)
	str.mu.Unlock()
	return nil
}

func (str *schedulerTestReconciler) snapshot() []string {
	str.mu.Lock()
	defer str.mu.Unlock()
	return append([]string(nil), str.modules...)
}
