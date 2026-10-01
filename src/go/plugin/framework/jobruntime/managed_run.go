// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// RunFailure is an operational collector outcome, not an ownership failure.
// Its classification is independent of its sanitized diagnostic text.
type RunFailure struct {
	cause      error
	class      collectorapi.LifecycleErrorClass
	retry      bool
	afterReady bool
	reason     string
}

func (e *RunFailure) Error() string { return e.cause.Error() }
func (e *RunFailure) Unwrap() error { return e.cause }

// Class is the collector's classification of the Run error.
func (e *RunFailure) Class() collectorapi.LifecycleErrorClass { return e.class }

// Retryable reports whether startup may be retried: an ordinary startup error
// or timeout before readiness that the collector did not classify permanent.
func (e *RunFailure) Retryable() bool  { return e.retry && !e.afterReady }
func (e *RunFailure) AfterReady() bool { return e.afterReady }
func (e *RunFailure) Reason() string   { return e.reason }

func newRunFailure(err error, reason string, sanitize func(error) error) *RunFailure {
	class := collectorapi.ClassifyLifecycleError(err)
	retry := (reason == "error" || reason == "startup_timeout") && class != collectorapi.LifecycleErrorPermanent
	return &RunFailure{cause: sanitizeLifecycleError(sanitize, err), class: class, retry: retry, reason: reason}
}

// ManagedRun serializes readiness, startup cancellation and collector termination.
// Logical settlement does not mean that the managed loop or its resources exited.
// onFailure MUST only revoke admissions; it runs under the settlement lock and
// MUST NOT wait, re-enter ManagedRun, or perform I/O.
type ManagedRun struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelCauseFunc
	startup    chan struct{}
	failed     chan struct{}
	settled    bool
	ready      bool
	startupErr error
	failure    *RunFailure
	running    atomic.Bool
	onFailure  func()
}

func NewManagedRun(ctx context.Context, onFailure func()) *ManagedRun {
	ctx, cancel := context.WithCancelCause(ctx)
	return &ManagedRun{ctx: ctx, cancel: cancel, startup: make(chan struct{}), failed: make(chan struct{}), onFailure: onFailure}
}

func (r *ManagedRun) Context() context.Context     { return r.ctx }
func (r *ManagedRun) StartupDone() <-chan struct{} { return r.startup }
func (r *ManagedRun) Failed() <-chan struct{}      { return r.failed }
func (r *ManagedRun) Running() bool                { return r.running.Load() }

// Ready is safe to call repeatedly or after cancellation. Only the first
// accepted startup outcome can enable collection.
func (r *ManagedRun) Ready() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settled {
		return
	}
	if cause := context.Cause(r.ctx); cause != nil {
		r.settleLocked(cause)
		return
	}
	r.ready = true
	r.running.Store(true)
	r.settleLocked(nil)
}

func (r *ManagedRun) StartupErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startupErr
}

func (r *ManagedRun) Failure() *RunFailure {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failure
}

// Stop ends logical service immediately, without waiting for physical teardown.
func (r *ManagedRun) Stop(cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cause == nil {
		cause = context.Canceled
	}
	r.running.Store(false)
	r.cancel(cause)
	if !r.settled {
		r.settleLocked(cause)
	}
}

// Timeout only applies to startup. It cannot stop an already-ready runtime.
func (r *ManagedRun) Timeout() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.settled {
		r.failLocked(newRunFailure(errors.New("collector startup timed out"), "startup_timeout", nil))
	}
}

// Complete is called at the collector boundary, independently of Collect.
// Requested cancellation normalizes only cancellation-only results.
func (r *ManagedRun) Complete(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running.Store(false)
	if r.failure != nil {
		return
	}
	if r.ctx.Err() != nil && (err == nil || cancellationOnly(err, r.ctx.Err(), context.Cause(r.ctx))) {
		if !r.settled {
			r.settleLocked(context.Cause(r.ctx))
		}
		return
	}
	failure, ok := errors.AsType[*RunFailure](err)
	if !ok {
		reason := "error"
		if err == nil {
			reason = "unexpected_return"
			err = errors.New("collector Run returned without a stop request")
		}
		failure = newRunFailure(err, reason, nil)
	}
	r.failLocked(failure)
}

func (r *ManagedRun) failLocked(failure *RunFailure) {
	copy := *failure
	copy.afterReady = r.ready
	r.failure = &copy
	r.running.Store(false)
	if r.onFailure != nil {
		r.onFailure()
	}
	r.cancel(r.failure)
	if !r.settled {
		r.settleLocked(r.failure)
	}
	close(r.failed)
}

func (r *ManagedRun) settleLocked(err error) {
	r.settled, r.startupErr = true, err
	close(r.startup)
}

func cancellationOnly(err error, canceled ...error) bool {
	if err == nil {
		return false
	}
	// Classification wrappers (especially recovered panic) are never cancellation.
	if _, ok := err.(*RunFailure); ok {
		return false
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		leaves := many.Unwrap()
		if len(leaves) == 0 {
			return false
		}
		for _, leaf := range leaves {
			if !cancellationOnly(leaf, canceled...) {
				return false
			}
		}
		return true
	}
	if one, ok := err.(interface{ Unwrap() error }); ok && one.Unwrap() != nil {
		return cancellationOnly(one.Unwrap(), canceled...)
	}
	for _, cause := range canceled {
		if cause != nil && errors.Is(err, cause) {
			return true
		}
	}
	return false
}
