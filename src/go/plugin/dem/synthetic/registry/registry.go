// SPDX-License-Identifier: GPL-3.0-or-later

// Package registry indexes the active synthetic jobs for Functions. Each job's monitor session owns its state;
// the registry keeps no copy of it.
package registry

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
)

// Job is one active job as Functions list it.
type Job struct {
	// Target is the credential-free URL of a URL-targeted kind; empty otherwise.
	Target string
	monitor.Snapshot
}

// Registry contains active jobs only. Native configuration owns desired, disabled
// and failed-startup jobs; Functions do not mirror their configs.
type Registry struct {
	mu   sync.Mutex
	jobs map[string]*registration
}

type registration struct {
	session *monitor.Session
	target  string
}

func New() *Registry {
	return &Registry{
		jobs: make(map[string]*registration),
	}
}

// Register makes an active session visible until the returned retire function runs. A job ID admits one session
// at a time; retiring an earlier session never removes its successor.
func (h *Registry) Register(session *monitor.Session, target string) (retire func(), err error) {
	jobID := session.JobID()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.jobs[jobID] != nil {
		return nil, errors.New("synthetic job already active")
	}
	r := &registration{
		session: session,
		target:  target,
	}
	h.jobs[jobID] = r
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.jobs[jobID] == r {
			delete(h.jobs, jobID)
		}
	}, nil
}

// Snapshot lists the active jobs at now, ordered by job ID.
func (h *Registry) Snapshot(now time.Time) []Job {
	h.mu.Lock()
	registrations := make([]*registration, 0, len(h.jobs))
	for _, r := range h.jobs {
		registrations = append(registrations, r)
	}
	h.mu.Unlock()
	out := make([]Job, len(registrations))
	for i, r := range registrations {
		out[i] = Job{
			Target:   r.target,
			Snapshot: r.session.Snapshot(now),
		}
	}
	slices.SortFunc(out, func(a, b Job) int { return strings.Compare(a.JobID, b.JobID) })
	return out
}

// Latest returns a copy of an active job's latest run when its ID is runID.
func (h *Registry) Latest(jobID, runID string) (synthetic.Run, bool) {
	h.mu.Lock()
	r := h.jobs[jobID]
	h.mu.Unlock()
	if r == nil {
		return synthetic.Run{}, false
	}
	return r.session.Latest(runID)
}

// HasJobs reports whether any job is active.
func (h *Registry) HasJobs() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.jobs) != 0
}
