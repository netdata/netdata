//go:build freebsd && (amd64 || 386)

// FreeBSD SHM primitives: _umtx_op wait/wake, CLOCK_MONOTONIC and the
// reserving allocation.

package posix

import (
	"syscall"
	"unsafe"
)

// _umtx_op operations. The shared (not _PRIVATE) ones key on the mapped
// object, so they cross processes.
const (
	umtxOpWake     = 3
	umtxOpWaitUint = 11
)

const shmClockMonotonic = 4

// umtxTime is struct _umtx_time: a relative timeout on a clock.
type umtxTime struct {
	timeout syscall.Timespec
	flags   uint32
	clockid uint32
}

// shmAllocate reserves the whole region file with posix_fallocate. ZFS
// rejects it with EINVAL: SHM preparation fails and the session uses
// baseline.
func shmAllocate(fd int, mode uint32, off, length int64) error {
	_ = mode
	if errno := posixFallocate(fd, off, length); errno != 0 {
		return errno
	}
	return nil
}

func futexWakeCall(data []byte, off int, count int) int {
	if off < 0 || off+4 > len(data) {
		return -1
	}
	addr := unsafe.Pointer(&data[off]) // #nosec G103 -- _umtx_op needs the address of the mmap signal word.
	r1, _, errno := syscall.Syscall6(syscall.SYS__UMTX_OP,
		uintptr(addr), umtxOpWake, uintptr(count), 0, 0, 0)
	if errno != 0 {
		return -int(errno)
	}
	return int(r1)
}

// futexWaitCall waits while the word equals expected. A stale expected
// value returns 0 at once (a Linux futex returns EAGAIN); callers re-check
// the sequence after every return.
func futexWaitCall(data []byte, off int, expected uint32, ts *syscall.Timespec) int {
	if off < 0 || off+4 > len(data) {
		return -1
	}
	addr := unsafe.Pointer(&data[off]) // #nosec G103 -- _umtx_op needs the address of the mmap signal word.
	var size, tp uintptr
	var t umtxTime
	if ts != nil {
		t = umtxTime{timeout: *ts, clockid: shmClockMonotonic}
		size = unsafe.Sizeof(t)
		tp = uintptr(unsafe.Pointer(&t)) // #nosec G103 -- _umtx_op takes an optional _umtx_time pointer.
	}
	r1, _, errno := syscall.Syscall6(syscall.SYS__UMTX_OP,
		uintptr(addr), umtxOpWaitUint, uintptr(expected), size, tp, 0)
	if errno != 0 {
		return -int(errno)
	}
	return int(r1)
}
