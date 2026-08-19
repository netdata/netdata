//go:build unix

package raw

import (
	"syscall"
	"unsafe"
)

// poll constants (not exported by Go's syscall package)
const (
	_POLLIN   = 0x0001
	_POLLERR  = 0x0008
	_POLLHUP  = 0x0010
	_POLLNVAL = 0x0020
)

// pollfd matches struct pollfd from <poll.h>.
type pollfd struct {
	fd      int32
	events  int16
	revents int16
}

// pollFd polls a file descriptor for readability with a timeout in ms.
// Returns: 1 = data ready, 0 = timeout, -1 = error/hangup.
func pollFd(fd int, timeoutMs int) int {
	const maxInt32 = 1<<31 - 1
	if fd < 0 || fd > maxInt32 {
		return -1
	}
	pfd := pollfd{
		fd:     int32(fd), // #nosec G115 -- fd is checked against int32 range above.
		events: _POLLIN,
	}

	r, _, errno := syscall.Syscall(
		syscall.SYS_POLL,
		uintptr(unsafe.Pointer(&pfd)), // #nosec G103 -- raw poll syscall requires a pollfd pointer.
		1,
		uintptr(timeoutMs),
	)

	n := int(r)
	if n < 0 {
		if errno == syscall.EINTR {
			return 0
		}
		return -1
	}

	if n == 0 {
		return 0
	}

	if pfd.revents&(_POLLERR|_POLLHUP|_POLLNVAL) != 0 {
		return -1
	}

	if pfd.revents&_POLLIN != 0 {
		return 1
	}

	return 0
}
