//go:build linux

package main

import (
	"syscall"
	"unsafe"
)

// clockBoottime is CLOCK_BOOTTIME from uapi/linux/time.h: monotonic since boot,
// and unlike CLOCK_MONOTONIC it keeps advancing while the machine is suspended.
// That is the safer choice here — a plugin restarted after a resume must not
// produce tokens below the watermark it left before the suspend.
const clockBoottime = 7

// bootNanosFromClock reads CLOCK_BOOTTIME directly.  It is preferred over
// /proc/uptime because it has nanosecond rather than centisecond resolution, so
// two process lifetimes that start within the same /proc/uptime tick still get
// strictly ordered references.
//
// The raw syscall is used so this works with CGO_ENABLED=0 and without pulling
// golang.org/x/sys into the plugin's module.  A failure (for example a 32-bit
// arch that only offers clock_gettime64) is not fatal: the caller falls back to
// /proc/uptime, which is the same boot-relative domain.
func bootNanosFromClock() (uint64, bool) {
	var ts syscall.Timespec
	if _, _, errno := syscall.Syscall(
		uintptr(syscall.SYS_CLOCK_GETTIME),
		clockBoottime,
		uintptr(unsafe.Pointer(&ts)),
		0,
	); errno != 0 {
		return 0, false
	}
	if ts.Sec < 0 || ts.Nsec < 0 {
		return 0, false
	}

	return uint64(ts.Sec)*uint64(1_000_000_000) + uint64(ts.Nsec), true
}
