// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// StagedRestarts is one store command's dependent-restart plan, staged by
// the manager on the run loop before the effect ships:
//
//   - Run executes the restarts inside the effect and returns the terminal
//     message plus a non-nil error when the DEADLINE cut the sequence short
//     (dependents skipped because the command's time ran out). The error
//     makes the classification race-independent: a cut sequence must reach
//     the command's failure outcome even when the effect returns before
//     the worker observes the expired deadline - a nil return would let
//     the completion-vs-abandon select race decide between success and
//     the timeout failure for the same physical state.
//   - Flush replays the completed restarts' loop-side entry-state and
//     CONFIG STATUS transitions, in restart order, draining as it goes.
//     Each completed restart replays exactly once: at the command's commit,
//     or - for restarts that complete only after a deadline abandon - at
//     the late return, while the dependents' claims are still held.
type StagedRestarts struct {
	Run   func(ctx context.Context) (string, error)
	Flush func()
}

// storeCommandRun is one secretstore command's effect-to-commit carrier. It
// rides the command's effect contexts, so concurrent store commands can
// never cross-attribute restart messages or replay buffers. The mutex
// covers effect-goroutine writes vs loop-side reads.
type storeCommandRun struct {
	staged *StagedRestarts

	mu      sync.Mutex
	message string
	// activated flips when the service took the new value: the custom
	// commit paths (add, conversion) use it to classify a plain failure by
	// PHASE - pre-activation (validation-class, coded fallbacks apply) vs
	// applied-but-degraded (the restart sequence was cut or failed after
	// activation, which answers 200 with the failure text like the shared
	// update path). The shared handler needs no marker - its phases commit
	// separately.
	activated bool
}

func (r *storeCommandRun) setMessage(msg string) {
	r.mu.Lock()
	r.message = msg
	r.mu.Unlock()
}

// markActivated records that the service holds the new value; called by the
// mutating callbacks on the effect goroutine right after service.Update/Add
// succeed.
func (r *storeCommandRun) markActivated() {
	r.mu.Lock()
	r.activated = true
	r.mu.Unlock()
}

func (r *storeCommandRun) activatedNow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activated
}

// commandCode maps a store mutation's failure to its terminal code with
// phase awareness, mirroring the shared handler's update mapping: coded
// errors keep their codes; a plain error AFTER activation is the
// applied-but-degraded partial outcome and answers 200 with the failure
// text (terminal 400 would misreport an applied mutation as a bad
// request); a plain error before activation falls back to the
// validation-class codes.
func (r *storeCommandRun) commandCode(err error) int {
	var ce interface{ DyncfgCode() int }
	if errors.As(err, &ce) {
		return ce.DyncfgCode()
	}
	if r.activatedNow() {
		return 200
	}
	return secretStoreErrorCode(err)
}

func (r *storeCommandRun) takeMessage() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg := strings.TrimSpace(r.message)
	r.message = ""
	return msg
}

// flush replays the completed dependent restarts. Loop-side only.
func (r *storeCommandRun) flush() {
	if r.staged != nil {
		r.staged.Flush()
	}
}

type storeCommandRunKey struct{}

func withStoreCommandRun(ctx context.Context, r *storeCommandRun) context.Context {
	return context.WithValue(ctx, storeCommandRunKey{}, r)
}

func storeCommandRunFrom(ctx context.Context) *storeCommandRun {
	r, _ := ctx.Value(storeCommandRunKey{}).(*storeCommandRun)
	return r
}

// newStoreCommandRun builds the command's carrier and stages its
// dependent-restart plan (on the caller's goroutine - the run loop) when the
// command addresses a derivable store key.
func (c *Controller) newStoreCommandRun(fn dyncfg.Function) *storeCommandRun {
	r := &storeCommandRun{}
	if c.stageDependentRestarts == nil {
		return r
	}
	if key, _, ok := c.cb.ExtractKey(fn); ok {
		r.staged = c.stageDependentRestarts(key)
	}
	return r
}

// storeStepRunner wraps a StepRunner so every effect of the command carries
// its run box and every commit first replays the buffered per-job work -
// except never-ran commits, which publish nothing by the shutdown rule.
func (c *Controller) storeStepRunner(run dyncfg.StepRunner, r *storeCommandRun) dyncfg.StepRunner {
	return func(effect func(context.Context) error, commit func(error)) {
		run(func(ctx context.Context) error {
			return effect(withStoreCommandRun(ctx, r))
		}, func(err error) {
			if !errors.Is(err, dyncfg.ErrPhaseNeverRan) {
				r.flush()
			}
			commit(err)
		})
	}
}
