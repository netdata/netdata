//go:build !linux

package main

// bootNanosFromClock has no portable equivalent outside Linux.  The caller falls
// back to /proc/uptime and then to 0; both stay in the boot-relative domain.
func bootNanosFromClock() (uint64, bool) { return 0, false }
