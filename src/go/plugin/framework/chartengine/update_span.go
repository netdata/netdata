// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "time"

// sampleStamp is a plan time as an update-time stamp: unix nanoseconds, zero
// when the time is unset.
func sampleStamp(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	if n := t.UnixNano(); n > 0 {
		return uint64(n)
	}
	return 0
}

// sinceLastUpdate is an update's span: the time since the chart's previous
// update, zero when either stamp is unknown or the clock went backwards.
func sinceLastUpdate(at, last uint64) time.Duration {
	if at == 0 || last == 0 || at <= last {
		return 0
	}
	return time.Duration(at - last)
}
