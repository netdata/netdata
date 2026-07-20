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
	mu     sync.Mutex
	tickMu sync.Mutex

	reconciler ModuleReconciler
	retries    *autoDetectionRetryIndex
	jobs       map[lifecycle.ResourceIdentity]*scheduledJob
	modules    map[string]*scheduledModule
	jobHead    *scheduledJob
	jobTail    *scheduledJob
	moduleHead *scheduledModule
	moduleTail *scheduledModule
	moduleTick []string
}

type scheduledJob struct {
	identity lifecycle.ResourceIdentity
	job      RuntimeJob
	previous *scheduledJob
	next     *scheduledJob
}

type scheduledModule struct {
	name     string
	jobs     int
	previous *scheduledModule
	next     *scheduledModule
}

func NewScheduler(reconciler ModuleReconciler) (*Scheduler, error) {
	if reconciler == nil {
		return nil, errors.New("job output: scheduler has no module reconciler")
	}
	return &Scheduler{
		reconciler: reconciler,
		retries:    newAutoDetectionRetryIndex(),
		jobs:       make(map[lifecycle.ResourceIdentity]*scheduledJob),
		modules:    make(map[string]*scheduledModule),
	}, nil
}

func (s *Scheduler) bindAutoDetectionRetries(
	commands jobmgr.PreparedCommandPort,
	plan autoDetectionRetryPlanner,
	run uint64,
) error {
	if s == nil {
		return errors.New(
			"job output: nil autodetection retry scheduler",
		)
	}
	return s.retries.bind(commands, plan, run)
}

func (s *Scheduler) CloseAutoDetectionRetries() {
	if s != nil {
		s.retries.close()
	}
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
	record := &scheduledJob{
		identity: identity,
		job:      job,
		previous: s.jobTail,
	}
	if s.jobTail == nil {
		s.jobHead = record
	} else {
		s.jobTail.next = record
	}
	s.jobTail = record
	s.jobs[identity] = record

	module := s.modules[job.ModuleName()]
	if module == nil {
		module = &scheduledModule{
			name:     job.ModuleName(),
			previous: s.moduleTail,
		}
		if s.moduleTail == nil {
			s.moduleHead = module
		} else {
			s.moduleTail.next = module
		}
		s.moduleTail = module
		s.modules[module.name] = module
	}
	module.jobs++
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
	record := s.jobs[identity]
	if record == nil || record.job != job {
		return errors.New("job output: stale scheduler unregistration")
	}
	module := s.modules[job.ModuleName()]
	if module == nil || module.jobs <= 0 {
		return errors.New("job output: scheduler module census differs")
	}
	if record.previous == nil {
		s.jobHead = record.next
	} else {
		record.previous.next = record.next
	}
	if record.next == nil {
		s.jobTail = record.previous
	} else {
		record.next.previous = record.previous
	}
	delete(s.jobs, identity)

	module.jobs--
	if module.jobs == 0 {
		if module.previous == nil {
			s.moduleHead = module.next
		} else {
			module.previous.next = module.next
		}
		if module.next == nil {
			s.moduleTail = module.previous
		} else {
			module.next.previous = module.previous
		}
		delete(s.modules, module.name)
	}
	return nil
}

func (s *Scheduler) Tick(ctx context.Context, clock int) error {
	if s == nil || ctx == nil {
		return errors.New("job output: invalid scheduler tick")
	}
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	if err := s.retries.dispatchDue(ctx, clock); err != nil {
		return err
	}
	s.mu.Lock()
	for record := s.jobHead; record != nil; record = record.next {
		record.job.Tick(clock)
	}
	s.moduleTick = s.moduleTick[:0]
	for module := s.moduleHead; module != nil; module = module.next {
		s.moduleTick = append(s.moduleTick, module.name)
	}
	s.mu.Unlock()

	var result error
	for index, module := range s.moduleTick {
		s.moduleTick[index] = ""
		if err := s.reconciler.ReconcileModule(ctx, module); err != nil {
			result = errors.Join(result, err)
		}
	}
	s.moduleTick = s.moduleTick[:0]
	return result
}

func (s *Scheduler) Census() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}
