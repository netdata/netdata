// SPDX-License-Identifier: GPL-3.0-or-later

package schedulers

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
)

// SchedulerJobHandle identifies a job attached through a SchedulerRegistry.
type SchedulerJobHandle struct {
	scheduler  string
	jobID      string
	generation uint64
}

// SchedulerRegistry provides injected, non-global scheduler ownership.
type SchedulerRegistry interface {
	Ensure(def Definition, log *logger.Logger) error
	Remove(name string) error
	Attach(name string, reg runtime.JobRegistration, log *logger.Logger) (*SchedulerJobHandle, error)
	Detach(handle *SchedulerJobHandle)
	Collect(name string) map[string]int64
	Snapshot(name string) (runtime.SchedulerSnapshot, bool)
	Get(name string) (Definition, bool)
	All() []Definition
}

type schedulerHost interface {
	stop()
	attach(reg runtime.JobRegistration) (string, error)
	detach(jobID string)
	collectMetrics() map[string]int64
	collectSnapshot() runtime.SchedulerSnapshot
	jobCount() int
	snapshotJobs() map[string]runtime.JobRegistration
	restoreJobs(jobs map[string]runtime.JobRegistration) error
}

type schedulerHostFactory interface {
	New(def Definition, log *logger.Logger) (schedulerHost, error)
}

type runtimeSchedulerHostFactory struct{}

func (f runtimeSchedulerHostFactory) New(def Definition, log *logger.Logger) (schedulerHost, error) {
	return newRuntimeHost(def, log)
}

type registryEntry struct {
	def        Definition
	host       schedulerHost
	generation uint64
}

// Registry is an injectable scheduler registry with no package-global state.
type Registry struct {
	mu          sync.RWMutex
	defs        map[string]Definition
	entries     map[string]*registryEntry
	hostFactory schedulerHostFactory
}

// NewRegistry creates a new scheduler registry seeded with the builtin default definition.
func NewRegistry() *Registry {
	return newRegistryWithFactory(runtimeSchedulerHostFactory{})
}

func newRegistryWithFactory(factory schedulerHostFactory) *Registry {
	if factory == nil {
		factory = runtimeSchedulerHostFactory{}
	}
	r := &Registry{
		defs:        make(map[string]Definition),
		entries:     make(map[string]*registryEntry),
		hostFactory: factory,
	}
	r.defs["default"] = defaultDefinition()
	return r
}

// Ensure registers or updates a scheduler definition.
//
// Locked incompatibility predicate (30.3.A):
// changes to workers, queue_size, logging emitter wiring, or builtin flag are rejected.
func (r *Registry) Ensure(def Definition, log *logger.Logger) error {
	if def.Name == "" {
		return fmt.Errorf("scheduler name is required")
	}
	norm := normalizeDefinition(def)

	r.mu.Lock()
	defer r.mu.Unlock()

	existingDef, hasDef := r.defs[norm.Name]
	if !hasDef {
		host, err := r.hostFactory.New(norm, log)
		if err != nil {
			return err
		}
		r.defs[norm.Name] = norm
		r.entries[norm.Name] = &registryEntry{
			def:        norm,
			host:       host,
			generation: 0,
		}
		return nil
	}

	if !definitionsCompatible(existingDef, norm) {
		return fmt.Errorf("scheduler '%s' definition is incompatible with existing runtime-impacting fields", norm.Name)
	}

	entry, hasEntry := r.entries[norm.Name]
	if !hasEntry {
		host, err := r.hostFactory.New(norm, log)
		if err != nil {
			return err
		}
		r.defs[norm.Name] = norm
		r.entries[norm.Name] = &registryEntry{
			def:        norm,
			host:       host,
			generation: 0,
		}
		return nil
	}

	jobs := entry.host.snapshotJobs()
	newHost, err := r.hostFactory.New(norm, log)
	if err != nil {
		return err
	}
	if err := newHost.restoreJobs(jobs); err != nil {
		newHost.stop()
		return err
	}

	oldHost := entry.host
	entry.host = newHost
	entry.def = norm
	entry.generation++
	r.defs[norm.Name] = norm

	// Stop old host after successful swap.
	oldHost.stop()

	return nil
}

// Remove deletes a scheduler definition if no jobs are attached.
//
// Locked default behavior (30.4.A):
// removing "default" resets builtin definition instead of deleting it.
func (r *Registry) Remove(name string) error {
	if name == "" {
		return fmt.Errorf("scheduler name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry := r.entries[name]
	if entry != nil && entry.host.jobCount() > 0 {
		return fmt.Errorf("scheduler '%s' still has %d jobs", name, entry.host.jobCount())
	}
	if entry != nil {
		entry.host.stop()
		delete(r.entries, name)
	}

	if name == "default" {
		r.defs[name] = defaultDefinition()
		return nil
	}
	delete(r.defs, name)
	return nil
}

// Attach registers a job with the named scheduler.
func (r *Registry) Attach(name string, reg runtime.JobRegistration, log *logger.Logger) (*SchedulerJobHandle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := r.entries[name]
	if entry == nil {
		def, ok := r.defs[name]
		if !ok {
			return nil, fmt.Errorf("scheduler '%s' not defined", name)
		}
		host, err := r.hostFactory.New(def, log)
		if err != nil {
			return nil, err
		}
		entry = &registryEntry{def: def, host: host, generation: 0}
		r.entries[name] = entry
	}

	jobID, err := entry.host.attach(reg)
	if err != nil {
		return nil, err
	}
	return &SchedulerJobHandle{
		scheduler:  name,
		jobID:      jobID,
		generation: entry.generation,
	}, nil
}

// Detach removes a job. Stale generation handles are ignored.
func (r *Registry) Detach(handle *SchedulerJobHandle) {
	if handle == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry := r.entries[handle.scheduler]
	if entry == nil {
		return
	}
	if handle.generation != entry.generation {
		return
	}
	entry.host.detach(handle.jobID)
}

// Collect returns current runtime metrics for one scheduler.
func (r *Registry) Collect(name string) map[string]int64 {
	r.mu.RLock()
	entry := r.entries[name]
	r.mu.RUnlock()
	if entry == nil {
		return nil
	}
	return entry.host.collectMetrics()
}

// Snapshot returns a typed scheduler snapshot for v2 collectors.
func (r *Registry) Snapshot(name string) (runtime.SchedulerSnapshot, bool) {
	r.mu.RLock()
	entry := r.entries[name]
	r.mu.RUnlock()
	if entry == nil {
		return runtime.SchedulerSnapshot{}, false
	}
	return entry.host.collectSnapshot(), true
}

// Get returns a normalized definition by name.
func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	def, ok := r.defs[name]
	r.mu.RUnlock()
	if !ok {
		return Definition{}, false
	}
	return normalizeDefinition(def), true
}

// All returns all normalized definitions.
func (r *Registry) All() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.defs))
	for _, def := range r.defs {
		out = append(out, normalizeDefinition(def))
	}
	return out
}

func definitionsCompatible(a, b Definition) bool {
	return a.Name == b.Name &&
		a.Workers == b.Workers &&
		a.QueueSize == b.QueueSize &&
		a.LoggingEnabled == b.LoggingEnabled &&
		a.Builtin == b.Builtin &&
		reflect.DeepEqual(normalizeDefinition(a).Logging, normalizeDefinition(b).Logging)
}
