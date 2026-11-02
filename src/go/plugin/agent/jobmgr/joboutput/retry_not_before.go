// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"math"
	"time"
)

// Cadence grants ordinary recovery; provider metadata constrains any later
// automatic activation of this candidate, even without ordinary recovery.
type candidateRetryPolicy struct {
	cadence   int64
	notBefore time.Time
}

func (f *autoDetectionFailure) retryPolicy() candidateRetryPolicy {
	policy := candidateRetryPolicy{notBefore: f.notBefore}
	if f.retry && f.retryAfter > 0 {
		policy.cadence = int64(f.retryAfter)
	}
	return policy
}

func (e *candidateEntry) clearRetry() { e.HasDue = false }

// resetRetry is reserved for explicit manual acceptance/fresh enablement.
func (e *candidateEntry) resetRetry() {
	e.clearRetry()
	e.NotBefore = time.Time{}
}

func (e *candidateEntry) retainMinimum(deadline time.Time) {
	if e.NotBefore.Before(deadline) {
		e.NotBefore = deadline
	}
}

func (e *candidateEntry) setRetry(policy candidateRetryPolicy, now int64) {
	e.HasDue = policy.cadence > 0
	e.retainMinimum(policy.notBefore)
	e.Due = retryLogicalDue(now, policy.cadence)
}

// All new automatic activations share this gate. A satisfied minimum is consumed
// at admission; physical continuation does not retroactively reopen that wait.
func (e *candidateEntry) providerReady(wallNow time.Time) bool {
	return !wallNow.Before(e.NotBefore)
}

func (e *candidateEntry) consumeMinimum() { e.NotBefore = time.Time{} }

func (e *candidateEntry) retryReady(logicalNow int64, wallNow time.Time) bool {
	return e.HasDue && e.Due <= logicalNow && e.providerReady(wallNow)
}

// The existing queue carries estimates, never authority to bypass providerReady.
func (e *candidateEntry) providerWake(logicalNow int64, wallNow time.Time) int64 {
	remaining := e.NotBefore.Sub(wallNow)
	if remaining <= 0 {
		return logicalNow
	}
	seconds := int64(remaining / time.Second)
	if remaining%time.Second != 0 {
		seconds++
	}
	return retryLogicalDue(logicalNow, seconds)
}

func (e *candidateEntry) retryWake(logicalNow int64, wallNow time.Time) int64 {
	if e.providerReady(wallNow) {
		return e.Due
	}
	return max(e.Due, e.providerWake(logicalNow, wallNow))
}

func retryLogicalDue(now, delay int64) int64 {
	if delay > math.MaxInt64-now {
		return math.MaxInt64
	}
	return now + delay
}

// Deferred source selection is an unconsumed inventory transition, not an
// executor. Automatic admission may advance Intent without consuming it;
// explicit intent resets revoke it together with executable work.
type candidateDeferredSource struct {
	Observation sourceObservation
}

// Selection reads source authority without mutating pending state or its wake.
func (r *candidateRecord) validDeferredSourceEntry() *candidateEntry {
	pending := r.DeferredSource
	if pending == nil {
		return nil
	}
	e := r.entry(pending.Observation.Key)
	lifetime, present := r.Inventory.Sources[pending.Observation.Key.Source]
	if !present || lifetime != pending.Observation.Lifetime || e == nil || !e.Observed || !e.SourceOwned || e.Rejected ||
		e.SourceLifetime != pending.Observation.Lifetime || !candidateAuthorized(r, e.Key) {
		return nil
	}
	return e
}

func (r *candidateRecord) deferredSourceEntry() *candidateEntry {
	e := r.validDeferredSourceEntry()
	if e == nil && r.DeferredSource != nil {
		r.cancelDeferredSource()
	}
	return e
}

// Removing source authority must not discard an independent ordinary hint.
func (r *candidateRecord) cancelDeferredSource() {
	r.DeferredSource = nil
	if r.Wake != nil && r.Wake.Source {
		r.Wake = nil
	}
}

// Every timed preparation finalizes the pending source hint after its final
// phase/ownership changes, including outcomes that return without progress.
func (c *candidateController) finalizeRetryDelta(result **candidateDelta, resultErr *error, now int64) {
	d := *result
	if d != nil && *resultErr == nil && !d.Decision.Ignored {
		r := d.after
		var due int64
		scheduled, source := false, false
		if r.Wake != nil && !r.Wake.Source {
			due, scheduled = r.Wake.Due, true
		}
		if e := r.deferredSourceEntry(); e != nil && r.Phase != candidateBlocked {
			if hint := e.providerWake(now, c.wallNow()); !scheduled || hint < due {
				due, scheduled, source = hint, true, true
			}
		}
		if err := c.installWake(d, due, scheduled, source); err != nil {
			*result, *resultErr = nil, err
			return
		}
	}
	finalizeCandidateDelta(result)
}
