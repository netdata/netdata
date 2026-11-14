// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// User job names cannot collide with internal qualification work.
type executionKey struct {
	jobID         string
	qualification bool
}

type admissionWaiter struct {
	ready   chan struct{}
	job     *admissionJob
	granted bool
}

// admissionJob queues one logical job's requests. It is in the owner's ready
// queue exactly when it has waiters and no active attempt.
type admissionJob struct {
	key     executionKey
	active  bool
	waiters []*admissionWaiter
}

// executionOwner shares browser.concurrency slots fairly across jobs, per
// Agent. The ready queue holds jobs, not requests: each release gives the next
// waiting job one turn and requeues a job with remaining requests at the tail.
// Queues stay short (a job waits with its monitor call plus explicit
// rehearsals; the ready queue has at most one entry per configured job), so
// slices with linear removal on cancellation are enough.
type executionOwner struct {
	mu         sync.Mutex
	limit      int
	active     int
	jobs       map[executionKey]*admissionJob
	ready      []*admissionJob
	fatal      chan error
	poisoned   chan struct{}
	poisonOnce sync.Once
}

func newExecutionOwner(limit int) *executionOwner {
	return &executionOwner{
		limit:    limit,
		jobs:     make(map[executionKey]*admissionJob),
		fatal:    make(chan error, 1),
		poisoned: make(chan struct{}),
	}
}

func (o *executionOwner) acquire(ctx context.Context, key executionKey) (*admissionJob, error) {
	o.mu.Lock()
	if err := ctx.Err(); err != nil {
		o.mu.Unlock()
		return nil, err
	}
	if o.isPoisoned() {
		o.mu.Unlock()
		return nil, ndexec.ErrTreeNotDrained
	}
	job := o.jobs[key]
	if job == nil {
		job = &admissionJob{key: key}
		o.jobs[key] = job
	}
	waiter := &admissionWaiter{ready: make(chan struct{}), job: job}
	job.waiters = append(job.waiters, waiter)
	if !job.active && len(job.waiters) == 1 {
		o.ready = append(o.ready, job)
	}
	o.dispatch()
	o.mu.Unlock()
	select {
	case <-waiter.ready:
		return job, nil
	case <-ctx.Done():
		o.remove(waiter)
		return nil, ctx.Err()
	case <-o.poisoned:
		o.remove(waiter)
		return nil, ndexec.ErrTreeNotDrained
	}
}

func (o *executionOwner) dispatch() {
	for o.active < o.limit && len(o.ready) != 0 && !o.isPoisoned() {
		job := o.ready[0]
		o.ready = slices.Delete(o.ready, 0, 1)
		waiter := job.waiters[0]
		job.waiters = slices.Delete(job.waiters, 0, 1)
		waiter.granted = true
		job.active = true
		o.active++
		close(waiter.ready)
	}
}

// remove withdraws a waiter that stopped waiting. A grant that raced the stop
// is released instead.
func (o *executionOwner) remove(waiter *admissionWaiter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if waiter.granted {
		o.releaseLocked(waiter.job)
		return
	}
	job := waiter.job
	job.waiters = slices.DeleteFunc(job.waiters, func(w *admissionWaiter) bool { return w == waiter })
	if len(job.waiters) == 0 && !job.active {
		o.ready = slices.DeleteFunc(o.ready, func(j *admissionJob) bool { return j == job })
		delete(o.jobs, job.key)
	}
}

func (o *executionOwner) release(job *admissionJob) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.releaseLocked(job)
}

func (o *executionOwner) releaseLocked(job *admissionJob) {
	job.active = false
	o.active--
	if len(job.waiters) == 0 {
		delete(o.jobs, job.key)
	} else {
		o.ready = append(o.ready, job)
	}
	o.dispatch()
}

func (o *executionOwner) isPoisoned() bool { return closed(o.poisoned) }
