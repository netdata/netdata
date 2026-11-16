//go:build unix && !(linux && 386)

package posix

import (
	"syscall"
	"unsafe"
)

// rawSendmsg issues sendmsg(2) directly so header and payload go out as one
// packet without copying them into a single buffer.
func rawSendmsg(fd int, msg *syscall.Msghdr, flags int) (uintptr, syscall.Errno) {
	n, _, errno := syscall.Syscall(
		syscall.SYS_SENDMSG,
		uintptr(fd),
		uintptr(unsafe.Pointer(msg)), // #nosec G103 -- raw sendmsg syscall requires a Msghdr pointer.
		uintptr(flags),
	)
	return n, errno
}
