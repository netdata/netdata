//go:build linux

// Linux SHM primitives: futex wait/wake and the reserving allocation.

package posix

import (
	"syscall"
	"unsafe"
)

// futex operations
const (
	futexWait = 0
	futexWake = 1
)

// shmAllocate reserves the whole region file.
func shmAllocate(fd int, mode uint32, off, length int64) error {
	return syscall.Fallocate(fd, mode, off, length)
}

func futexWakeCall(data []byte, off int, count int) int {
	if off < 0 || off+4 > len(data) {
		return -1
	}
	addr := unsafe.Pointer(&data[off]) // #nosec G103 -- futex needs the address of the mmap signal word.
	r1, _, _ := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(addr),
		uintptr(futexWake),
		uintptr(count),
		0, 0, 0,
	)
	return int(r1)
}

func futexWaitCall(data []byte, off int, expected uint32, ts *syscall.Timespec) int {
	if off < 0 || off+4 > len(data) {
		return -1
	}
	addr := unsafe.Pointer(&data[off]) // #nosec G103 -- futex needs the address of the mmap signal word.
	var tsPtr uintptr
	if ts != nil {
		tsPtr = uintptr(unsafe.Pointer(ts)) // #nosec G103 -- futex syscall requires an optional Timespec pointer.
	}
	r1, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(addr),
		uintptr(futexWait),
		uintptr(expected),
		tsPtr,
		0, 0,
	)
	if errno != 0 {
		return -int(errno)
	}
	return int(r1)
}
