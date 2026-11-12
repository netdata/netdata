//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"time"

	"golang.org/x/sys/unix"
)

func monotonicNow() (time.Duration, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return 0, err
	}
	// The supported Linux Node runtime's hrtime uses this same clock. Other
	// platforms use it in unit tests; runtimeAllowed still rejects execution.
	return time.Duration(ts.Nano()), nil
}
