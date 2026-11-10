// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"container/list"
	"context"
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
	element *list.Element
	job     *admissionJob
	granted bool
}

type admissionJob struct {
	key     executionKey
	active  bool
	waiters list.List
	ready   *list.Element
}

// The ready list contains eligible jobs, not individual requests. Each release
// gives a waiting job one turn before placing its remaining requests at the tail.
// All mutations, including cancellation, are O(1) per granted or removed request.
type executionOwner struct {
	mu         sync.Mutex
	limit      int
	active     int
	jobs       map[executionKey]*admissionJob
	ready      list.List
	fatal      chan error
	poisoned   chan struct{}
	poisonOnce sync.Once
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
	waiter.element = job.waiters.PushBack(waiter)
	if !job.active && job.ready == nil {
		job.ready = o.ready.PushBack(job)
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
	for o.active < o.limit && o.ready.Len() != 0 && !o.isPoisoned() {
		job := o.ready.Remove(o.ready.Front()).(*admissionJob)
		job.ready = nil
		waiter := job.waiters.Remove(job.waiters.Front()).(*admissionWaiter)
		waiter.element = nil
		waiter.granted = true
		job.active = true
		o.active++
		close(waiter.ready)
	}
}

func (o *executionOwner) remove(waiter *admissionWaiter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if waiter.granted {
		o.releaseLocked(waiter.job)
		return
	}
	job := waiter.job
	job.waiters.Remove(waiter.element)
	if job.waiters.Len() == 0 {
		if job.ready != nil {
			o.ready.Remove(job.ready)
			job.ready = nil
		}
		if !job.active {
			delete(o.jobs, job.key)
		}
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
	if job.waiters.Len() == 0 {
		delete(o.jobs, job.key)
	} else {
		job.ready = o.ready.PushBack(job)
	}
	o.dispatch()
}

func (o *executionOwner) isPoisoned() bool {
	select {
	case <-o.poisoned:
		return true
	default:
		return false
	}
}
