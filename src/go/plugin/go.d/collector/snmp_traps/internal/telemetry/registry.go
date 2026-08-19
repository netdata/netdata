// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import "sync"

// Registry owns the lifecycle of job telemetry handles. Recording and
// collection use the handles directly and never consult the registry.
type Registry struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewRegistry() *Registry {
	return &Registry{jobs: make(map[string]*Job)}
}

type Options struct {
	DedupEnabled bool
}

// Attach creates a fresh handle for one job lifecycle. A later lifecycle with
// the same name replaces the registry entry without invalidating the old
// handle; identity-aware Detach keeps stale cleanup from removing the new one.
func (r *Registry) Attach(jobName string, opts Options) *Job {
	job := &Job{
		name:         jobName,
		registry:     r,
		dedupEnabled: opts.DedupEnabled,
	}
	r.mu.Lock()
	r.jobs[jobName] = job
	r.mu.Unlock()
	return job
}

func (r *Registry) Detach(job *Job) {
	if r == nil || job == nil {
		return
	}
	r.mu.Lock()
	if r.jobs[job.name] == job {
		delete(r.jobs, job.name)
	}
	r.mu.Unlock()
}
