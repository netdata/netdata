// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package snmp_traps

import (
	"time"

	"golang.org/x/sys/unix"
)

func monotonicUsec() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return time.Now().UnixMicro()
	}
	return int64(ts.Sec)*1_000_000 + int64(ts.Nsec)/1_000
}
