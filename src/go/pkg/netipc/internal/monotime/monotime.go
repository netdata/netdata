// Package monotime reads Go's monotonic clock without the wall clock.
//
// time.Now reads both clocks. Where the platform has no user-space clock
// (FreeBSD on a kvmclock timecounter) each read is a syscall, so per-call
// deadlines use this package instead: time.Since on a time that carries a
// monotonic reading reads only the monotonic clock.
package monotime

import "time"

var base = time.Now()

// Now returns the monotonic time since process start, in nanoseconds.
func Now() int64 {
	return int64(time.Since(base))
}

// After returns the monotonic time d from now, in nanoseconds. For d > 0
// the result is never zero, so callers may use zero as "no deadline".
func After(d time.Duration) int64 {
	return Now() + int64(d)
}
