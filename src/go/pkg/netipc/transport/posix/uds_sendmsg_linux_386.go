//go:build linux && 386

package posix

import (
	"syscall"
	"unsafe"
)

// socketcall(2) multiplexer number for sendmsg; linux/386 has no direct
// sendmsg system call. Mirrors syscall_linux_386.go in the Go runtime.
const socketcallSendmsg = 16

// rawSendmsg issues sendmsg(2) through socketcall(2) so header and payload
// go out as one packet without copying them into a single buffer.
func rawSendmsg(fd int, msg *syscall.Msghdr, flags int) (uintptr, syscall.Errno) {
	args := [3]uintptr{
		uintptr(fd),
		uintptr(unsafe.Pointer(msg)), // #nosec G103 -- socketcall sendmsg requires a Msghdr pointer.
		uintptr(flags),
	}
	n, _, errno := syscall.Syscall(
		syscall.SYS_SOCKETCALL,
		socketcallSendmsg,
		uintptr(unsafe.Pointer(&args)), // #nosec G103 -- socketcall takes a pointer to its argument array.
		0,
	)
	return n, errno
}
