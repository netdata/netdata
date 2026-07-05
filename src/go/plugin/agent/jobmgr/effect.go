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

	// lateWork, set by a closure with loop-side work that must still run
	// when the effect is abandoned (a store command's dependent-restart
	// replay for restarts that complete only after the deadline), is
	// delivered on the late completion and executed by the loop while the
	// command's write claims are still held.
	lateWork atomic.Pointer[func()]
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

func (c *effectControl) setLateWork(fn func()) {
	if c != nil {
		c.lateWork.Store(&fn)
	}
}

func (c *effectControl) takeLateWork() func() {
	if c == nil {
		return nil
	}
	if p := c.lateWork.Load(); p != nil {
		return *p
	}
	return nil
}

func (c *effectControl) setQuarantine(fn func()) {
	if c != nil {
		c.quarantine.Store(&fn)
	}
}

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

type effectOutcome uint8

const (
	effectOutcomeCompleted effectOutcome = iota
	effectOutcomeAbandoned
	effectOutcomeLateReturn
	effectOutcomeShutdownNeverRan
)

type effectResult struct {
	key     string
	outcome effectOutcome
	err     error
	resume  *warmResume
	// lateWork is loop-side work delivered with a late completion (replay
	// of dependent restarts that completed after the abandon); it runs
	// before the command's remaining claims release.
	lateWork func()
	// delivered, set on abandoned results, is closed by the worker AFTER
	// the result reaches the loop (or is dropped): the leaked child's late
	// completion gates on it, so a call that returns instantly at
	// cancellation can never overtake the abandon - an overtaken late
	// result would be dropped as not-wedged and the key would wedge with
	// nothing left to release it.
	delivered chan struct{}
	busyFor   time.Duration
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
				res = effectResult{key: t.key, outcome: effectOutcomeShutdownNeverRan, err: dyncfg.ErrPhaseNeverRan}
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
			if res.delivered != nil {
				close(res.delivered)
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
		return effectResult{key: t.key, outcome: effectOutcomeCompleted, err: err, busyFor: time.Since(started)}
	case <-ctx.Done():
		if !ctl.claimAbandon() {
			// The closure claimed completion first: its in-process post-work
			// is finishing - wait it out and report a normal completion.
			err := <-done
			cancel()
			return effectResult{key: t.key, outcome: effectOutcomeCompleted, err: err}
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
		abandonDelivered := make(chan struct{})
		go func() {
			defer m.leakedChildren.Done()
			err := <-done
			cancel()
			m.leakedNow.Add(-1)
			// The abandoned result must reach the loop first: an instantly
			// returning leaked call would otherwise race its late completion
			// ahead of the abandon.
			select {
			case <-abandonDelivered:
			case <-m.lateDrop:
			}
			res := effectResult{
				key:      t.key,
				outcome:  effectOutcomeLateReturn,
				err:      err,
				resume:   ctl.resume.Load(),
				lateWork: ctl.takeLateWork(),
			}
			if r := res.resume; r != nil && m.ctx.Err() != nil {
				// Shutdown is in effect: no warm job will ever start, and a
				// buffered send can win the race against the closed lateDrop
				// and strand the result unread - dispose here (silently, one
				// rule), deliver a resume-less completion (a consumer just
				// unwedges the key).
				m.disposeWarmResume(r)
				res.resume = nil
			}
			if res.lateWork != nil && m.ctx.Err() != nil {
				// One rule at shutdown: nothing publishes after the drain -
				// the late replay is dropped, not run.
				res.lateWork = nil
			}
			select {
			case m.effectDoneCh <- res:
			case <-m.lateDrop:
				m.Warningf("dropping late effect completion for key '%s' after shutdown (err: %v)", t.key, err)
				if r := res.resume; r != nil {
					// Unreachable once the pre-send disposal ran (shutdown
					// precedes lateDrop's close); kept as a safety net.
					m.disposeWarmResume(r)
				}
			}
		}()
		var abandonErr error
		switch {
		case errors.Is(context.Cause(ctx), context.Canceled):
			// Shutdown cancelled the effect context (dyncfg deadlines carry a
			// timeout cause; commands are never cancelled individually). ONE
			// RULE: every non-terminal command at shutdown answers 503,
			// publishes nothing, and disposes everything - regardless of
			// which phase was running, how far it got, or whether a fence
			// ran. This is the worker-side choke point; the callback-side
			// conversions cover the child winning the completion claim.
			abandonErr = fmt.Errorf("interrupted by shutdown: %w", dyncfg.ErrPhaseNeverRan)
		case fenced:
			abandonErr = fmt.Errorf("%w: timed out after %s (the collector call is still running)", dyncfg.ErrPhaseAbandoned, m.effectDeadline)
		default:
			abandonErr = fmt.Errorf("operation timed out after %s and was abandoned (the collector call is still running)", m.effectDeadline)
		}
		return effectResult{
			key:       t.key,
			outcome:   effectOutcomeAbandoned,
			err:       abandonErr,
			delivered: abandonDelivered,
			busyFor:   time.Since(started),
		}
	}
}
