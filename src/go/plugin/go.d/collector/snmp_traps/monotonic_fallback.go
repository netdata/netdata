// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package snmp_traps

func monotonicUsec() int64 {
	return defaultMonotonicUsec()
}
