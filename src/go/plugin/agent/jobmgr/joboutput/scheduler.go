// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type ModuleReconciler interface {
	ReconcileModule(context.Context, string) error
}

type Scheduler struct {
	mu     sync.Mutex
	tickMu sync.Mutex

	reconciler ModuleReconciler
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
		jobs:       make(map[lifecycle.ResourceIdentity]*scheduledJob),
		modules:    make(map[string]*scheduledModule),
	}, nil
}

func (scheduler *Scheduler) Register(
	identity lifecycle.ResourceIdentity,
	job RuntimeJob,
) error {
	if scheduler == nil || !identity.Valid() || job == nil ||
		identity.ID != job.FullName() || job.ModuleName() == "" {
		return errors.New("job output: invalid scheduler registration")
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.jobs[identity] != nil {
		return errors.New("job output: duplicate scheduler registration")
	}
	record := &scheduledJob{
		identity: identity,
		job:      job,
		previous: scheduler.jobTail,
	}
	if scheduler.jobTail == nil {
		scheduler.jobHead = record
	} else {
		scheduler.jobTail.next = record
	}
	scheduler.jobTail = record
	scheduler.jobs[identity] = record

	module := scheduler.modules[job.ModuleName()]
	if module == nil {
		module = &scheduledModule{
			name:     job.ModuleName(),
			previous: scheduler.moduleTail,
		}
		if scheduler.moduleTail == nil {
			scheduler.moduleHead = module
		} else {
			scheduler.moduleTail.next = module
		}
		scheduler.moduleTail = module
		scheduler.modules[module.name] = module
	}
	module.jobs++
	return nil
}

func (scheduler *Scheduler) Unregister(
	identity lifecycle.ResourceIdentity,
	job RuntimeJob,
) error {
	if scheduler == nil || !identity.Valid() || job == nil {
		return errors.New("job output: invalid scheduler unregistration")
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	record := scheduler.jobs[identity]
	if record == nil || record.job != job {
		return errors.New("job output: stale scheduler unregistration")
	}
	module := scheduler.modules[job.ModuleName()]
	if module == nil || module.jobs <= 0 {
		return errors.New("job output: scheduler module census differs")
	}
	if record.previous == nil {
		scheduler.jobHead = record.next
	} else {
		record.previous.next = record.next
	}
	if record.next == nil {
		scheduler.jobTail = record.previous
	} else {
		record.next.previous = record.previous
	}
	delete(scheduler.jobs, identity)

	module.jobs--
	if module.jobs == 0 {
		if module.previous == nil {
			scheduler.moduleHead = module.next
		} else {
			module.previous.next = module.next
		}
		if module.next == nil {
			scheduler.moduleTail = module.previous
		} else {
			module.next.previous = module.previous
		}
		delete(scheduler.modules, module.name)
	}
	return nil
}

func (scheduler *Scheduler) Tick(ctx context.Context, clock int) error {
	if scheduler == nil || ctx == nil {
		return errors.New("job output: invalid scheduler tick")
	}
	scheduler.tickMu.Lock()
	defer scheduler.tickMu.Unlock()
	scheduler.mu.Lock()
	for record := scheduler.jobHead; record != nil; record = record.next {
		record.job.Tick(clock)
	}
	scheduler.moduleTick = scheduler.moduleTick[:0]
	for module := scheduler.moduleHead; module != nil; module = module.next {
		scheduler.moduleTick = append(scheduler.moduleTick, module.name)
	}
	scheduler.mu.Unlock()

	var result error
	for index, module := range scheduler.moduleTick {
		scheduler.moduleTick[index] = ""
		if err := scheduler.reconciler.ReconcileModule(ctx, module); err != nil {
			result = errors.Join(result, err)
		}
	}
	scheduler.moduleTick = scheduler.moduleTick[:0]
	return result
}

func (scheduler *Scheduler) Census() int {
	if scheduler == nil {
		return 0
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return len(scheduler.jobs)
}
