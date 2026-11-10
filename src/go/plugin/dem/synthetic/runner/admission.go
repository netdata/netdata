// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"container/list"
	"context"
	"sync"
	"time"

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
	scope   *browserSource
	granted bool
}

type admissionJob struct {
	key     executionKey
	slot    *browserSlot
	waiters list.List
	ready   *list.Element
}

// The ready list contains eligible jobs, not individual requests. Each release
// gives a waiting job one turn before placing its remaining requests at the tail.
type executionOwner struct {
	mu         sync.Mutex
	limit      int
	active     int
	jobs       map[executionKey]*admissionJob
	ready      list.List
	idle       list.List
	idleFor    time.Duration
	stopping   bool
	closing    chan struct{}
	closed     chan struct{}
	closeOnce  sync.Once
	busy       sync.WaitGroup
	fatal      chan error
	poisoned   chan struct{}
	poisonOnce sync.Once
}

type browserSlot struct {
	owner *browserOwner
	idle  *list.Element
	timer *time.Timer
}

func (o *executionOwner) acquire(ctx context.Context, key executionKey, scope *browserSource) (*admissionJob, error) {
	o.mu.Lock()
	if err := ctx.Err(); err != nil {
		o.mu.Unlock()
		return nil, err
	}
	if o.isPoisoned() {
		o.mu.Unlock()
		return nil, ndexec.ErrTreeNotDrained
	}
	if o.stopping {
		o.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-o.poisoned:
			return nil, ndexec.ErrTreeNotDrained
		case <-o.closing:
			return nil, context.Canceled
		}
	}
	job := o.jobs[key]
	if job == nil {
		job = &admissionJob{key: key}
		o.jobs[key] = job
	}
	waiter := &admissionWaiter{ready: make(chan struct{}), job: job, scope: scope}
	waiter.element = job.waiters.PushBack(waiter)
	if job.slot == nil && job.ready == nil {
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
	case <-o.closing:
		o.remove(waiter)
		return nil, context.Canceled
	}
}

func (o *executionOwner) dispatch() {
	for o.active < o.limit && o.ready.Len() != 0 && !o.stopping && !o.isPoisoned() {
		job := o.ready.Remove(o.ready.Front()).(*admissionJob)
		job.ready = nil
		waiter := job.waiters.Remove(job.waiters.Front()).(*admissionWaiter)
		waiter.element = nil
		waiter.granted = true
		job.slot = o.takeSlot(waiter.scope)
		o.active++
		o.busy.Add(1)
		close(waiter.ready)
	}
}

// Admission chooses the job first. Only that job can prefer a compatible warm
// slot, or leave an unrelated idle owner alone when empty capacity is available.
func (o *executionOwner) takeSlot(scope *browserSource) *browserSlot {
	first := o.idle.Front()
	if scope != nil {
		for entry := first; entry != nil; entry = entry.Next() {
			slot := entry.Value.(*browserSlot)
			if slot.owner != nil && slot.owner.scope == scope && slot.owner.alive() {
				o.removeIdle(slot)
				return slot
			}
		}
	}
	if first == nil || (first.Value.(*browserSlot).owner != nil && o.active+o.idle.Len() < o.limit) {
		return &browserSlot{}
	}
	slot := first.Value.(*browserSlot)
	o.removeIdle(slot)
	return slot
}

func (o *executionOwner) removeIdle(slot *browserSlot) {
	o.idle.Remove(slot.idle)
	slot.idle = nil
	if slot.timer != nil {
		slot.timer.Stop()
		slot.timer = nil
	}
}

func (o *executionOwner) makeIdle(slot *browserSlot) {
	if slot.owner == nil {
		slot.idle = o.idle.PushFront(slot)
		return
	}
	slot.idle = o.idle.PushBack(slot)
	select {
	case <-o.closing:
		o.retireIdle(slot)
		return
	default:
	}
	var timer *time.Timer
	timer = time.AfterFunc(o.idleFor, func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if slot.timer == timer {
			o.retireIdle(slot)
		}
	})
	slot.timer = timer
}

// An idle owner being retired occupies its slot until its supervisor joins.
func (o *executionOwner) retireIdle(slot *browserSlot) {
	o.removeIdle(slot)
	o.active++
	o.busy.Add(1)
	go func() {
		defer o.busy.Done()
		if result := slot.owner.retire(); !result.Drained {
			return
		}
		o.mu.Lock()
		defer o.mu.Unlock()
		slot.owner = nil
		o.active--
		o.makeIdle(slot)
		o.dispatch()
	}()
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
		if job.slot == nil {
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
	slot := job.slot
	job.slot = nil
	o.active--
	o.makeIdle(slot)
	o.busy.Done()
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
