// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package snmp_traps

import "time"

var monotonicStarted = time.Now()

func monotonicUsec() int64 {
	return time.Since(monotonicStarted).Microseconds()
}
