// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
)

// An effect is a unit of blocking module work (config validation, job
// creation, detection, stop) executed off the manager loop in a supervised
// child goroutine. Completions are delivered through effectDoneCh.
//
// The current execution policy is fully synchronous: runEffectSync spawns
// the child and immediately drains its completion, so at most one effect is
// ever in flight and the observable sequence is identical to calling the
// closure inline. The effect context carries no deadline and is never
// cancelled.
//
// Discipline: runEffectSync must be called only from the goroutine that owns
// manager state - the run loop in production. It preserves panic semantics:
// a child panic is re-raised on the caller goroutine.

type effectResult struct {
	key        string
	err        error
	panicked   bool
	panicValue any
}

// runEffectSync executes run in a supervised child goroutine and waits for
// its completion. The completion travels over a per-call channel, NOT
// effectDoneCh: callers are not always the run loop (tests drive manager
// entry points from other goroutines while the loop runs), and any shared
// completion channel would race the loop's effectDoneCh select arms.
func (m *Manager) runEffectSync(key string, run func(ctx context.Context) error) error {
	// Deadline disabled: the effect context is uncancellable, matching the
	// blocking behavior of the inline calls it replaces.
	ctx := context.Background()

	done := make(chan effectResult, 1)
	go func() {
		res := effectResult{key: key}
		defer func() {
			if r := recover(); r != nil {
				res.panicked = true
				res.panicValue = r
			}
			done <- res
		}()
		res.err = run(ctx)
	}()

	res := <-done
	if res.panicked {
		// Today a panic on this path crashes the plugin; keep that contract.
		panic(res.panicValue)
	}
	return res.err
}

// handleUnexpectedEffectDone is the loop-select arm for effect completions.
// Under the synchronous policy every completion is drained by its
// dispatcher, so an arrival here is a bug, not an event to act on.
func (m *Manager) handleUnexpectedEffectDone(res effectResult) {
	m.Errorf("BUG: unexpected effect completion for key '%s' (err: %v) reached the run loop", res.key, res.err)
}
