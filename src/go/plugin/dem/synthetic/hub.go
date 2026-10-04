// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"errors"
	"sort"
	"sync"
	"time"
)

type Job struct {
	JobID          string  `json:"job_id"`
	Kind           Kind    `json:"kind"`
	Name           string  `json:"name"`
	Target         string  `json:"target,omitempty"`
	CadenceSeconds int     `json:"cadence_seconds"`
	TimeoutSeconds float64 `json:"timeout_seconds"`
	State          string  `json:"state"`
	Fresh          bool    `json:"fresh"`
	Latest         *Run    `json:"latest,omitempty"`
	LastSuccessUS  int64   `json:"last_success_us"`
	LastFailureUS  int64   `json:"last_failure_us"`
}

// Hub contains active observation snapshots only. Native configuration owns
// desired/disabled/failed-startup jobs; Functions do not mirror their configs.
type Hub struct {
	mu   sync.Mutex
	jobs map[string]*Registration
}

type Registration struct {
	hub     *Hub
	job     Job
	retired bool
}

func NewHub() *Hub { return &Hub{jobs: make(map[string]*Registration)} }
func (h *Hub) Register(job Job) (*Registration, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if job.JobID == "" {
		return nil, errors.New("empty synthetic job identity")
	}
	if h.jobs[job.JobID] != nil {
		return nil, errors.New("synthetic job already active")
	}
	job.State = "unknown"
	r := &Registration{hub: h, job: job}
	h.jobs[job.JobID] = r
	return r, nil
}
func (r *Registration) SetState(state string) {
	r.hub.mu.Lock()
	defer r.hub.mu.Unlock()
	if !r.retired {
		r.job.State = state
	}
}
func (r *Registration) Complete(run Run) {
	r.hub.mu.Lock()
	defer r.hub.mu.Unlock()
	if r.retired {
		return
	}
	r.job.Latest = cloneRun(&run)
	r.job.State = string(run.Outcome)
	if run.Outcome == Success {
		r.job.LastSuccessUS = run.CompletedUS
	}
	if run.Outcome == Failed || run.Outcome == Timeout {
		r.job.LastFailureUS = run.CompletedUS
	}
}
func (r *Registration) Retire() {
	r.hub.mu.Lock()
	defer r.hub.mu.Unlock()
	if r.retired {
		return
	}
	r.retired = true
	if r.hub.jobs[r.job.JobID] == r {
		delete(r.hub.jobs, r.job.JobID)
	}
}
func (h *Hub) Snapshot(now time.Time) []Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Job, 0, len(h.jobs))
	for _, r := range h.jobs {
		job := r.job
		job.Latest = cloneRun(job.Latest)
		if job.Latest != nil {
			age := now.Sub(time.UnixMicro(job.Latest.CompletedUS))
			job.Fresh = job.Latest.CompletedUS > 0 && age >= 0 && age <= 2*time.Duration(job.CadenceSeconds)*time.Second
			if job.Latest.CompletedUS > 0 && !job.Fresh && job.State != "waiting" && job.State != "running" {
				job.State = "stale"
			}
		}

		out = append(out, job)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JobID < out[j].JobID })
	return out
}
func cloneRun(in *Run) *Run {
	if in == nil {
		return nil
	}
	result := CloneRun(*in)
	return &result
}
