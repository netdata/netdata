// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"math"
	"time"
)

// candidateRetryPolicy keeps both constraints together through every failure
// handoff. Zero means recovery is disabled, regardless of provider metadata.
type candidateRetryPolicy struct {
	cadence   int64
	notBefore time.Time
}

func (f *autoDetectionFailure) retryPolicy() candidateRetryPolicy {
	if !f.retry || f.retryAfter <= 0 {
		return candidateRetryPolicy{}
	}
	return candidateRetryPolicy{cadence: int64(f.retryAfter), notBefore: f.notBefore}
}

func (e *candidateEntry) clearRetry() {
	e.HasDue = false
	e.NotBefore = time.Time{}
}

func (e *candidateEntry) setRetry(policy candidateRetryPolicy, now int64) {
	e.HasDue = policy.cadence > 0
	e.NotBefore = policy.notBefore
	e.Due = retryLogicalDue(now, policy.cadence)
}

// retryReady is the admission authority. Wake estimates never replace either
// the configured logical cadence or the absolute provider constraint.
func (e *candidateEntry) retryReady(logicalNow int64, wallNow time.Time) bool {
	return e.HasDue && e.Due <= logicalNow && !wallNow.Before(e.NotBefore)
}

// retryWake estimates a useful future tick. Early or compressed ticks are safe:
// the controller checks retryReady again, then rearms without starting work.
func (e *candidateEntry) retryWake(logicalNow int64, wallNow time.Time) int64 {
	due := e.Due
	if remaining := e.NotBefore.Sub(wallNow); remaining > 0 {
		seconds := int64(remaining / time.Second)
		if remaining%time.Second != 0 {
			seconds++
		}
		due = max(due, retryLogicalDue(logicalNow, seconds))
	}
	return due
}

func retryLogicalDue(now, delay int64) int64 {
	if delay > math.MaxInt64-now {
		return math.MaxInt64
	}
	return now + delay
}
