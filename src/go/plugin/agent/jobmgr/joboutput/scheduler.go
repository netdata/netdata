// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type ModuleReconciler interface {
	ReconcileModule(context.Context, string) error
}

type Scheduler struct {
	mu     sync.Mutex // guards jobs and modules
	tickMu sync.Mutex // serializes Tick() so ticks never overlap

	reconciler ModuleReconciler                          // per-module reconciliation callback
	retries    *autoDetectionRetryIndex                  // auto-detection retry worker
	jobs       map[lifecycle.ResourceIdentity]RuntimeJob // scheduled job by identity
	modules    map[string]int                            // scheduled job count by module
}

func NewScheduler(reconciler ModuleReconciler) (*Scheduler, error) {
	if reconciler == nil {
		return nil, errors.New("job output: scheduler has no module reconciler")
	}
	return &Scheduler{
		reconciler: reconciler,
		retries:    newAutoDetectionRetryIndex(),
		jobs:       make(map[lifecycle.ResourceIdentity]RuntimeJob),
		modules:    make(map[string]int),
	}, nil
}

func (s *Scheduler) bindAutoDetectionRetries(
	commands jobmgr.PreparedCommandPort,
	plan autoDetectionRetryPlanner,
	run uint64,
	failure func(error),
) error {
	if s == nil {
		return errors.New(
			"job output: nil autodetection retry scheduler",
		)
	}
	return s.retries.bind(commands, plan, run, failure)
}

func (s *Scheduler) StopAutoDetectionRetries() {
	if s != nil {
		s.retries.stopWorker()
	}
}

func (s *Scheduler) WaitAutoDetectionRetries(
	ctx context.Context,
) error {
	if s == nil {
		return errors.New(
			"job output: nil autodetection retry scheduler",
		)
	}
	return s.retries.wait(ctx)
}

func (s *Scheduler) AutoDetectionRetriesJoined() bool {
	return s == nil || s.retries.joined()
}

func (s *Scheduler) Register(
	identity lifecycle.ResourceIdentity,
	job RuntimeJob,
) error {
	if s == nil || !identity.Valid() || job == nil ||
		identity.ID != job.FullName() || job.ModuleName() == "" {
		return errors.New("job output: invalid scheduler registration")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs[identity] != nil {
		return errors.New("job output: duplicate scheduler registration")
	}
	s.jobs[identity] = job
	s.modules[job.ModuleName()]++
	return nil
}

func (s *Scheduler) Unregister(
	identity lifecycle.ResourceIdentity,
	job RuntimeJob,
) error {
	if s == nil || !identity.Valid() || job == nil {
		return errors.New("job output: invalid scheduler unregistration")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	registered := s.jobs[identity]
	if registered == nil || registered != job {
		return errors.New("job output: stale scheduler unregistration")
	}
	module := job.ModuleName()
	count := s.modules[module]
	if count <= 0 {
		return errors.New("job output: scheduler module census differs")
	}
	delete(s.jobs, identity)
	if count == 1 {
		delete(s.modules, module)
	} else {
		s.modules[module] = count - 1
	}
	return nil
}

func (s *Scheduler) Tick(ctx context.Context, clock int) error {
	if s == nil || ctx == nil {
		return errors.New("job output: invalid scheduler tick")
	}
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	s.retries.advance(clock)
	s.mu.Lock()
	// Keep registration stable until every selected job has been ticked. A
	// successful Unregister is the caller's authority to clean up the job.
	for _, job := range s.jobs {
		job.Tick(clock)
	}
	modules := make([]string, 0, len(s.modules))
	for module := range s.modules {
		modules = append(modules, module)
	}
	s.mu.Unlock()

	var result error
	for _, module := range modules {
		if err := s.reconciler.ReconcileModule(ctx, module); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}
