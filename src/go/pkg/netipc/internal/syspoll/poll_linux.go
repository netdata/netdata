//go:build linux

package syspoll

import (
	"syscall"
	"unsafe"
)

// RdHup reports a peer that closed its end (Linux-only POLLRDHUP).
const RdHup int16 = 0x2000

// Poll waits up to timeoutMs (negative waits indefinitely) for pfd.Events.
// It returns the poll result count and the raw errno.
//
// Linux uses ppoll because poll does not exist on newer architectures such
// as arm64, riscv64 and loong64.
func Poll(pfd *PollFd, timeoutMs int) (int, syscall.Errno) {
	var ts *syscall.Timespec
	if timeoutMs >= 0 {
		t := syscall.NsecToTimespec(int64(timeoutMs) * 1_000_000)
		ts = &t
	}
	r, _, errno := syscall.Syscall6(
		syscall.SYS_PPOLL,
		uintptr(unsafe.Pointer(pfd)), // #nosec G103 -- raw ppoll syscall requires a pollfd pointer.
		1,
		uintptr(unsafe.Pointer(ts)), // #nosec G103 -- raw ppoll syscall requires a timespec pointer.
		0,
		0,
		0,
	)
	return int(r), errno
}
