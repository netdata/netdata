// SPDX-License-Identifier: GPL-3.0-or-later

package schedulers

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/spec"
)

type fakeHostFactory struct {
	mu             sync.Mutex
	nextCreateErr  error
	nextRestoreErr error
}

func (f *fakeHostFactory) New(def Definition, _ *logger.Logger) (schedulerHost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.nextCreateErr != nil {
		err := f.nextCreateErr
		f.nextCreateErr = nil
		return nil, err
	}

	h := &fakeHost{
		def:        def,
		jobs:       make(map[string]runtime.JobRegistration),
		restoreErr: f.nextRestoreErr,
	}
	f.nextRestoreErr = nil
	return h, nil
}

type fakeHost struct {
	def        Definition
	jobs       map[string]runtime.JobRegistration
	restoreErr error
	stopped    bool
}

func (h *fakeHost) stop() { h.stopped = true }

func (h *fakeHost) attach(reg runtime.JobRegistration) (string, error) {
	id := fmt.Sprintf("job-%d", len(h.jobs)+1)
	reg.ID = id
	h.jobs[id] = reg
	return id, nil
}

func (h *fakeHost) detach(jobID string) {
	delete(h.jobs, jobID)
}

func (h *fakeHost) collectSnapshot() runtime.SchedulerSnapshot {
	jobs := make([]runtime.JobMetricsSnapshot, 0, len(h.jobs))
	for id, reg := range h.jobs {
		jobs = append(jobs, runtime.JobMetricsSnapshot{
			JobID:   id,
			JobName: reg.Spec.Name,
		})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].JobID < jobs[j].JobID })

	return runtime.SchedulerSnapshot{
		Scheduler: h.def.Name,
		Scheduled: int64(len(h.jobs)),
		Jobs:      jobs,
	}
}

func (h *fakeHost) jobCount() int { return len(h.jobs) }

func (h *fakeHost) snapshotJobs() map[string]runtime.JobRegistration {
	out := make(map[string]runtime.JobRegistration, len(h.jobs))
	for id, reg := range h.jobs {
		out[id] = reg
	}
	return out
}

func (h *fakeHost) restoreJobs(jobs map[string]runtime.JobRegistration) error {
	if h.restoreErr != nil {
		return h.restoreErr
	}
	for id, reg := range jobs {
		stored := reg
		stored.ID = id
		h.jobs[id] = stored
	}
	return nil
}

func testRegJobSpec(name string) spec.JobSpec {
	return spec.JobSpec{
		Name:             name,
		Plugin:           "/bin/true",
		CheckInterval:    time.Second,
		RetryInterval:    time.Second,
		Timeout:          time.Second,
		MaxCheckAttempts: 1,
	}
}

func TestRegistryEnsureRejectsIncompatibleRuntimeFields(t *testing.T) {
	tests := map[string]func(Definition) Definition{
		"workers change": func(d Definition) Definition {
			d.Workers++
			return d
		},
		"queue size change": func(d Definition) Definition {
			d.QueueSize++
			return d
		},
		"logging enabled change": func(d Definition) Definition {
			d.LoggingEnabled = !d.LoggingEnabled
			return d
		},
		"logging config change": func(d Definition) Definition {
			d.Logging.Endpoint = d.Logging.Endpoint + "/changed"
			return d
		},
		"builtin flag change": func(d Definition) Definition {
			d.Builtin = !d.Builtin
			return d
		},
	}

	base := Definition{
		Name:           "compat",
		Workers:        5,
		QueueSize:      10,
		LoggingEnabled: true,
		Logging: runtime.OTLPEmitterConfig{
			Endpoint: "http://otel:4317",
			Timeout:  time.Second,
			UseTLS:   false,
			Headers:  map[string]string{"x": "y"},
		},
		Builtin: false,
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			factory := &fakeHostFactory{}
			reg := newRegistryWithFactory(factory)
			if err := reg.Ensure(base, nil); err != nil {
				t.Fatalf("ensure base failed: %v", err)
			}
			if err := reg.Ensure(mutate(base), nil); err == nil {
				t.Fatalf("expected incompatible update error")
			}
		})
	}
}

func TestRegistryRemoveDefaultResetsBuiltinDefinition(t *testing.T) {
	reg := newRegistryWithFactory(&fakeHostFactory{})
	handle, err := reg.Attach("default", runtime.JobRegistration{Spec: testRegJobSpec("job1")}, nil)
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}

	if err := reg.Remove("default"); err == nil {
		t.Fatalf("expected remove to fail while jobs are attached")
	}

	reg.Detach(handle)
	if err := reg.Remove("default"); err != nil {
		t.Fatalf("remove default failed: %v", err)
	}

	def, ok := reg.Get("default")
	if !ok {
		t.Fatalf("expected default definition to exist")
	}
	if !def.Builtin || def.Workers != 50 || def.QueueSize != 128 {
		t.Fatalf("unexpected default definition after reset: %+v", def)
	}
}

func TestRegistryEnsureFailureKeepsOldHostAndJobs(t *testing.T) {
	factory := &fakeHostFactory{}
	reg := newRegistryWithFactory(factory)

	def := Definition{
		Name:           "keep",
		Workers:        2,
		QueueSize:      4,
		LoggingEnabled: true,
		Logging: runtime.OTLPEmitterConfig{
			Endpoint: "http://otel:4317",
			Timeout:  time.Second,
			UseTLS:   false,
			Headers:  map[string]string{},
		},
	}
	if err := reg.Ensure(def, nil); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	handle, err := reg.Attach("keep", runtime.JobRegistration{Spec: testRegJobSpec("job1")}, nil)
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}

	factory.mu.Lock()
	factory.nextRestoreErr = errors.New("restore failed")
	factory.mu.Unlock()

	compatible := def
	compatible.Labels = map[string]string{"site": "athens"}
	if err := reg.Ensure(compatible, nil); err == nil {
		t.Fatalf("expected ensure reconfigure failure")
	}

	if err := reg.Remove("keep"); err == nil {
		t.Fatalf("expected remove to fail, old host/jobs should remain active")
	}

	reg.Detach(handle)
	if err := reg.Remove("keep"); err != nil {
		t.Fatalf("remove after detach failed: %v", err)
	}
}

func TestRegistryDetachIgnoresStaleGenerationHandle(t *testing.T) {
	reg := newRegistryWithFactory(&fakeHostFactory{})
	def := Definition{
		Name:           "stale",
		Workers:        2,
		QueueSize:      4,
		LoggingEnabled: true,
		Logging: runtime.OTLPEmitterConfig{
			Endpoint: "http://otel:4317",
			Timeout:  time.Second,
			UseTLS:   false,
			Headers:  map[string]string{},
		},
	}
	if err := reg.Ensure(def, nil); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	handle, err := reg.Attach("stale", runtime.JobRegistration{Spec: testRegJobSpec("job1")}, nil)
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}

	compatible := def
	compatible.Labels = map[string]string{"rack": "r1"}
	if err := reg.Ensure(compatible, nil); err != nil {
		t.Fatalf("compatible ensure failed: %v", err)
	}

	// Stale handle from prior generation should be ignored.
	reg.Detach(handle)
	if err := reg.Remove("stale"); err == nil {
		t.Fatalf("expected remove to fail, stale detach must not remove active job")
	}

	// Detach using current generation should remove the job.
	reg.mu.RLock()
	entry := reg.entries["stale"]
	currentGen := entry.generation
	reg.mu.RUnlock()
	reg.Detach(&SchedulerJobHandle{
		scheduler:  "stale",
		jobID:      handle.jobID,
		generation: currentGen,
	})
	if err := reg.Remove("stale"); err != nil {
		t.Fatalf("remove after current-generation detach failed: %v", err)
	}
}
