// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"sync"
	"time"
)

const DefaultShutdownTimeout = 10 * time.Second

type ShutdownBudget struct {
	deadline time.Time
	clock    Clock
	ctx      *shutdownContext
	cancel   func()
	stop     chan struct{}
	done     chan struct{}
	finish   sync.Once
}

func newShutdownBudget(clock Clock, timeout time.Duration) (*ShutdownBudget, error) {
	if clock == nil || timeout <= 0 {
		return nil, errors.New("jobmgr shutdown budget: invalid clock or timeout")
	}
	deadline := clock.Now().Add(timeout)
	timer, cancel := clock.Arm(TimerKindShutdown, timeout)
	budget := &ShutdownBudget{
		deadline: deadline,
		clock:    clock,
		ctx:      &shutdownContext{deadline: deadline, done: make(chan struct{})},
		cancel:   cancel,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go func() {
		defer close(budget.done)
		select {
		case <-timer:
			budget.ctx.finish(context.DeadlineExceeded)
		case <-budget.stop:
		}
	}()
	return budget, nil
}

func (budget *ShutdownBudget) Context() context.Context {
	if budget == nil {
		return nil
	}
	return budget.ctx
}

func (budget *ShutdownBudget) Deadline() time.Time {
	if budget == nil {
		return time.Time{}
	}
	return budget.deadline
}

func (budget *ShutdownBudget) Expired() bool {
	return budget != nil && errors.Is(budget.ctx.Err(), context.DeadlineExceeded)
}

func (budget *ShutdownBudget) ExpireIfDue() bool {
	if budget == nil {
		return false
	}
	if !budget.clock.Now().Before(budget.deadline) {
		budget.ctx.finish(context.DeadlineExceeded)
	}
	return budget.Expired()
}

func (budget *ShutdownBudget) close() error {
	if budget == nil {
		return nil
	}
	budget.finish.Do(func() {
		budget.ExpireIfDue()
		budget.cancel()
		close(budget.stop)
		<-budget.done
		budget.ctx.finish(context.Canceled)
	})
	if budget.Expired() {
		return context.DeadlineExceeded
	}
	return nil
}

type shutdownContext struct {
	mu       sync.Mutex
	deadline time.Time
	done     chan struct{}
	err      error
}

func (ctx *shutdownContext) Deadline() (time.Time, bool) { return ctx.deadline, true }
func (ctx *shutdownContext) Done() <-chan struct{}       { return ctx.done }
func (ctx *shutdownContext) Value(any) any               { return nil }

func (ctx *shutdownContext) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.err
}

func (ctx *shutdownContext) finish(err error) {
	ctx.mu.Lock()
	if ctx.err == nil {
		ctx.err = err
		close(ctx.done)
	}
	ctx.mu.Unlock()
}
