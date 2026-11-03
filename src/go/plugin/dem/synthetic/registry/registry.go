// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type Job struct {
	JobID          string         `json:"job_id"`
	Kind           synthetic.Kind `json:"kind"`
	Name           string         `json:"name"`
	Target         string         `json:"target,omitempty"`
	CadenceSeconds int            `json:"cadence_seconds"`
	TimeoutSeconds float64        `json:"timeout_seconds"`
	State          string         `json:"state"`
	Fresh          bool           `json:"fresh"`
	Latest         *synthetic.Run `json:"latest,omitempty"`
	LastSuccessUS  int64          `json:"last_success_us"`
	LastFailureUS  int64          `json:"last_failure_us"`
}

// Registry contains active observation snapshots only. Native configuration owns
// desired/disabled/failed-startup jobs; Functions do not mirror their configs.
type Registry struct {
	mu   sync.Mutex
	jobs map[string]*Registration
}

type Registration struct {
	registry *Registry
	job      Job
	retired  bool
}

func New() *Registry {
	return &Registry{
		jobs: make(map[string]*Registration),
	}
}
func (h *Registry) Register(job Job) (*Registration, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if job.JobID == "" {
		return nil, errors.New("empty synthetic job identity")
	}
	if h.jobs[job.JobID] != nil {
		return nil, errors.New("synthetic job already active")
	}
	job.State = "unknown"
	r := &Registration{
		registry: h,
		job:      job,
	}
	h.jobs[job.JobID] = r
	return r, nil
}
func (r *Registration) SetState(state string) {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	if !r.retired {
		r.job.State = state
	}
}
func (r *Registration) Complete(run synthetic.Run) {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	if r.retired {
		return
	}
	r.job.Latest = cloneRun(&run)
	r.job.State = string(run.Outcome)
	if run.Outcome == synthetic.Success {
		r.job.LastSuccessUS = run.CompletedUS
	}
	if run.Outcome == synthetic.Failed || run.Outcome == synthetic.Timeout {
		r.job.LastFailureUS = run.CompletedUS
	}
}
func (r *Registration) Retire() {
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	if r.retired {
		return
	}
	r.retired = true
	if r.registry.jobs[r.job.JobID] == r {
		delete(r.registry.jobs, r.job.JobID)
	}
}
func (h *Registry) Snapshot(now time.Time) []Job {
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
func cloneRun(in *synthetic.Run) *synthetic.Run {
	if in == nil {
		return nil
	}
	result := synthetic.CloneRun(*in)
	return &result
}
