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

func (sb *ShutdownBudget) Context() context.Context {
	if sb == nil {
		return nil
	}
	return sb.ctx
}

func (sb *ShutdownBudget) Deadline() time.Time {
	if sb == nil {
		return time.Time{}
	}
	return sb.deadline
}

func (sb *ShutdownBudget) Expired() bool {
	return sb != nil && errors.Is(sb.ctx.Err(), context.DeadlineExceeded)
}

func (sb *ShutdownBudget) ExpireIfDue() bool {
	if sb == nil {
		return false
	}
	if !sb.clock.Now().Before(sb.deadline) {
		sb.ctx.finish(context.DeadlineExceeded)
	}
	return sb.Expired()
}

func (sb *ShutdownBudget) close() error {
	if sb == nil {
		return nil
	}
	sb.finish.Do(func() {
		sb.ExpireIfDue()
		sb.cancel()
		close(sb.stop)
		<-sb.done
		sb.ctx.finish(context.Canceled)
	})
	if sb.Expired() {
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
func (ctx *shutdownContext) Value(_ any) any             { return nil }

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
