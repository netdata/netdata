// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// An effect is one blocking phase of a command (config validation, job
// creation, detection, stop), executed by the effect pool off the run loop
// under a flat internal deadline. Completions travel over effectDoneCh; THE
// RUN LOOP IS THE ONLY READER.
//
// Deadline protocol: when an effect exceeds the deadline, the WORKER abandons
// it - the pool slot frees and the command commits its deadline outcome - but
// the module call keeps running in the supervised child and THE KEY STAYS
// BUSY until it returns. Exactly one side wins the completion claim (CAS):
// a child that loses skips its normal post-work and follows the late
// protocol (publish a warm-start continuation on success, clean up and
// evaluate retry eligibility on failure); a worker that loses waits out the
// child's in-process post-work and reports a normal completion. Late
// completions are delivered to the loop shutdown-safely: after manager
// shutdown they are dropped with a log instead of blocking forever.
//
// A panic inside an effect is contained by the supervised child and converts
// to a failed completion: the command commits its failure path instead of
// crashing the plugin, and the key is released.

// defaultEffectDeadline is the flat internal cap on one blocking phase.
// Internal constant by design; tests may shorten it per manager.
const defaultEffectDeadline = 120 * time.Second

const (
	effectStateRunning = iota
	effectStateCompleted
	effectStateAbandoned
)

// effectControl is the abandon-protocol channel between the worker and the
// effect closure, carried on the effect context.
type effectControl struct {
	state atomic.Int32

	// resume, set by a detection closure that lost the completion claim
	// after a SUCCESSFUL detection, carries the warm job for the loop's
	// late-start continuation.
	resume atomic.Pointer[warmResume]

	// quarantine, set by a stop closure before its blocking wait, fences the
	// stopping job's output; the worker runs it at the abandon moment,
	// BEFORE the deadline outcome commits.
	quarantine atomic.Pointer[func()]

	// disruptive, set by a multi-phase closure once it has irreversibly
	// mutated state (an update's old instance is stopped): a shutdown
	// interruption after this point must commit as a failure, never as
	// never-ran - a never-ran outcome would roll caches back to a state
	// that no longer exists.
	disruptive atomic.Bool
}

// markDisruptive records the point of no return for the running phase.
func (c *effectControl) markDisruptive() {
	if c != nil {
		c.disruptive.Store(true)
	}
}

// warmResume is a late-detection success: the already-detected job, started
// directly by the loop when the continuation is still valid. It carries the
// job's own emission gate so drop paths can deregister exactly it.
type warmResume struct {
	cfg  confgroup.Config
	job  runtimeJob
	gate *emissionGateway
}

// claimCompletion is called by closures after their blocking module call:
// false means the worker abandoned this effect and the closure must follow
// the late protocol instead of its normal post-work.
func (c *effectControl) claimCompletion() bool {
	if c == nil {
		return true
	}
	return c.state.CompareAndSwap(effectStateRunning, effectStateCompleted)
}

// abandonedNow reports whether the worker has already abandoned this effect
// (without claiming anything): multi-phase closures check it between phases
// so an abandoned stop never proceeds into creating a replacement.
func (c *effectControl) abandonedNow() bool {
	return c != nil && c.state.Load() == effectStateAbandoned
}

func (c *effectControl) claimAbandon() bool {
	return c.state.CompareAndSwap(effectStateRunning, effectStateAbandoned)
}

func (c *effectControl) setResume(r *warmResume) { c.resume.Store(r) }

func (c *effectControl) setQuarantine(fn func()) { c.quarantine.Store(&fn) }

// clearQuarantine disarms the fence once the guarded blocking wait finished
// normally, so a deadline in a LATER phase of the same effect cannot fence a
// job that already stopped cleanly.
func (c *effectControl) clearQuarantine() {
	if c != nil {
		c.quarantine.Store(nil)
	}
}

func (c *effectControl) takeQuarantine() func() {
	if p := c.quarantine.Load(); p != nil {
		return *p
	}
	return nil
}

type effectControlKey struct{}

// effectControlFrom returns the abandon-protocol handle, nil on synchronous
// paths (which can never be abandoned).
func effectControlFrom(ctx context.Context) *effectControl {
	if ctx == nil {
		return nil
	}
	ctl, _ := ctx.Value(effectControlKey{}).(*effectControl)
	return ctl
}

type effectResult struct {
	key string
	err error
	// abandoned: the deadline fired; the key must stay busy (wedged) until
	// the late completion arrives.
	abandoned bool
	// late: the leaked child returned; the key unwedges and resume (if any
	// and still valid) starts the warm job.
	late    bool
	resume  *warmResume
	busyFor time.Duration
}

func (m *Manager) runEffectWorker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case t := <-m.effectCh:
			var res effectResult
			if m.ctx.Err() != nil {
				// Shutdown races the pick: a queued phase grabbed after cancel
				// must NOT start (no new module calls at shutdown) - it
				// resolves never-ran, same as the drain resolves the rest of
				// the queue.
				res = effectResult{key: t.key, err: dyncfg.ErrPhaseNeverRan}
			} else {
				res = m.superviseEffect(t)
			}
			select {
			case m.effectDoneCh <- res:
			case <-m.lateDrop:
				// The drain window expired with the results channel full; the
				// force pass already committed this command's terminal.
				m.Warningf("dropping effect completion for key '%s' after shutdown (err: %v)", t.key, res.err)
			}
		}
	}
}

func (m *Manager) superviseEffect(t effectTask) effectResult {
	ctl := &effectControl{}
	// Effects carry the deadline AND the manager lifetime: shutdown cancels
	// them (dyncfg cancellation never does). Deadline-honoring module calls
	// then return promptly during the bounded shutdown drain.
	ctx, cancel := context.WithTimeout(m.baseContext(), m.effectDeadline)
	ctx = context.WithValue(ctx, effectControlKey{}, ctl)

	done := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			if r := recover(); r != nil {
				m.Errorf("effect panic (key '%s'): %v", t.key, r)
				if m.executorMetrics != nil {
					m.executorMetrics.effectPanics.Add(1)
				}
				if logger.Level.Enabled(slog.LevelDebug) {
					m.Errorf("STACK: %s", debug.Stack())
				}
				err = fmt.Errorf("internal error: effect panic: %v", r)
			}
			done <- err
		}()
		err = t.effect(ctx)
	}()

	started := time.Now()
	select {
	case err := <-done:
		cancel()
		return effectResult{key: t.key, err: err, busyFor: time.Since(started)}
	case <-ctx.Done():
		if !ctl.claimAbandon() {
			// The closure claimed completion first: its in-process post-work
			// is finishing - wait it out and report a normal completion.
			err := <-done
			cancel()
			return effectResult{key: t.key, err: err}
		}
		// Quarantine the stopping job's output BEFORE the deadline outcome
		// commits (the commit happens after this result reaches the loop).
		// The classification below depends on it: only a FENCED abandon may
		// carry ErrPhaseAbandoned - stop-shaped commits publish success on
		// that sentinel, and success without a fence would break
		// no-output-after-publish (an abandon can win before the closure
		// even reaches the stop that registers the fence).
		fenced := false
		if q := ctl.takeQuarantine(); q != nil {
			q()
			fenced = true
		}
		if m.executorMetrics != nil {
			m.executorMetrics.leakedEffects.Add(1)
			m.executorMetrics.wedgedKeys.Add(1)
		}
		m.Warningf("effect deadline (%s) exceeded for key '%s'; abandoning (the collector call keeps running leaked)", m.effectDeadline, t.key)

		m.leakedChildren.Add(1)
		m.leakedNow.Add(1)
		go func() {
			defer m.leakedChildren.Done()
			err := <-done
			cancel()
			m.leakedNow.Add(-1)
			res := effectResult{key: t.key, err: err, late: true, resume: ctl.resume.Load()}
			if r := res.resume; r != nil && m.ctx.Err() != nil {
				// Shutdown is in effect: no warm job will ever start, and a
				// buffered send can win the race against the closed lateDrop
				// and strand the result unread - dispose here, deliver a
				// resume-less completion (a consumer just unwedges the key).
				r.job.Cleanup()
				m.emissionGates.removeMatching(r.cfg.FullName(), r.gate)
				res.resume = nil
			}
			select {
			case m.effectDoneCh <- res:
			case <-m.lateDrop:
				m.Warningf("dropping late effect completion for key '%s' after shutdown (err: %v)", t.key, err)
				if r := res.resume; r != nil {
					// Unreachable once the pre-send disposal ran (shutdown
					// precedes lateDrop's close); kept as a safety net.
					r.job.Cleanup()
					m.emissionGates.removeMatching(r.cfg.FullName(), r.gate)
				}
			}
		}()
		var abandonErr error
		shutdown := errors.Is(context.Cause(ctx), context.Canceled)
		switch {
		case shutdown && fenced:
			// Shutdown cancelled the effect context; same abandon mechanics
			// (the worker must never block at shutdown), different cause.
			abandonErr = fmt.Errorf("%w: job manager is shutting down", dyncfg.ErrPhaseAbandoned)
		case shutdown && ctl.disruptive.Load():
			// Shutdown after the phase's point of no return (the update's
			// old instance is already stopped): a never-ran rollback would
			// resurrect caches for a state that no longer exists - commit
			// as a plain disruptive failure instead.
			abandonErr = fmt.Errorf("interrupted by shutdown after the operation began: the previous state was already torn down")
		case shutdown:
			// Unfenced shutdown interruption: no matter which side wins the
			// completion claim, the command's outcome is "interrupted, retry
			// after restart" - the never-ran shape (503, nothing published,
			// caches untouched/restored). This is the single choke point:
			// callback-side conversions cover the child winning the claim,
			// this branch covers the worker winning it.
			abandonErr = fmt.Errorf("interrupted by shutdown: %w", dyncfg.ErrPhaseNeverRan)
		case fenced:
			abandonErr = fmt.Errorf("%w: timed out after %s (the collector call is still running)", dyncfg.ErrPhaseAbandoned, m.effectDeadline)
		default:
			abandonErr = fmt.Errorf("operation timed out after %s and was abandoned (the collector call is still running)", m.effectDeadline)
		}
		return effectResult{
			key:       t.key,
			err:       abandonErr,
			abandoned: true,
			busyFor:   time.Since(started),
		}
	}
}
