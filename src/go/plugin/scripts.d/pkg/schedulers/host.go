// SPDX-License-Identifier: GPL-3.0-or-later

package schedulers

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
)

type runtimeHost struct {
	def    Definition
	log    *logger.Logger
	sched  *runtime.Scheduler
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	jobs   map[string]runtime.JobRegistration
}

func newRuntimeHost(def Definition, log *logger.Logger) (*runtimeHost, error) {
	if log == nil {
		log = logger.New().With("scheduler", def.Name)
	}
	cfg := runtime.SchedulerConfig{
		Logger:        log,
		Workers:       def.Workers,
		QueueCapacity: def.QueueSize,
		SchedulerName: def.Name,
	}
	sched, err := runtime.NewScheduler(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := sched.Start(ctx); err != nil {
		cancel()
		sched.Stop()
		return nil, err
	}
	return &runtimeHost{def: def, log: log, sched: sched, ctx: ctx, cancel: cancel, jobs: make(map[string]runtime.JobRegistration)}, nil
}

func (h *runtimeHost) stop() {
	h.cancel()
	h.sched.Stop()
}

func (h *runtimeHost) attach(reg runtime.JobRegistration) (string, error) {
	jobID, err := h.sched.RegisterJob(reg)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	stored := reg
	stored.ID = jobID
	h.jobs[jobID] = stored
	h.mu.Unlock()
	return jobID, nil
}

func (h *runtimeHost) detach(jobID string) {
	h.sched.UnregisterJob(jobID)
	h.mu.Lock()
	if h.jobs != nil {
		delete(h.jobs, jobID)
	}
	h.mu.Unlock()
}

func (h *runtimeHost) collectMetrics() map[string]int64 {
	return h.sched.CollectMetrics()
}

func (h *runtimeHost) jobCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.jobs)
}

func (h *runtimeHost) snapshotJobs() map[string]runtime.JobRegistration {
	h.mu.Lock()
	defer h.mu.Unlock()
	copy := make(map[string]runtime.JobRegistration, len(h.jobs))
	for id, reg := range h.jobs {
		copy[id] = reg
	}
	return copy
}

func (h *runtimeHost) restoreJobs(jobs map[string]runtime.JobRegistration) error {
	for id, reg := range jobs {
		stored := reg
		stored.ID = id
		if _, err := h.sched.RegisterJob(stored); err != nil {
			return err
		}
		h.mu.Lock()
		h.jobs[id] = stored
		h.mu.Unlock()
	}
	return nil
}
