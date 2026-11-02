// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import "time"

// retryDelay is evaluated after cleanup, just before reading the logical clock
// for the candidate outcome. Waiting during teardown consumes the provider wait.
func (f *autoDetectionFailure) retryDelay(now time.Time) int64 {
	if !f.retry || f.retryAfter <= 0 {
		return 0
	}
	delay := int64(f.retryAfter)
	if remaining := f.notBefore.Sub(now); remaining > 0 {
		seconds := int64(remaining / time.Second)
		if remaining%time.Second != 0 {
			seconds++
		}
		// pkg/ticker emits an unbuffered incrementing 1Hz counter aligned to
		// seconds. The next tick can be imminent: reserve one extra tick so
		// converting a fractional wait cannot request before the deadline.
		delay = max(delay, seconds+1)
	}
	return delay
}
