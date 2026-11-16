//go:build unix

package raw

import (
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/syspoll"
)

// pollFd polls a file descriptor for readability with a timeout in ms.
// Returns: 1 = data ready, 0 = timeout, -1 = error/hangup.
func pollFd(fd int, timeoutMs int) int {
	const maxInt32 = 1<<31 - 1
	if fd < 0 || fd > maxInt32 {
		return -1
	}
	pfd := syspoll.PollFd{
		Fd:     int32(fd), // #nosec G115 -- fd is checked against int32 range above.
		Events: syspoll.In,
	}

	n, errno := syspoll.Poll(&pfd, timeoutMs)
	if n < 0 {
		if errno == syscall.EINTR {
			return 0
		}
		return -1
	}

	if n == 0 {
		return 0
	}

	if pfd.Revents&(syspoll.Err|syspoll.Hup|syspoll.Nval) != 0 {
		return -1
	}

	if pfd.Revents&syspoll.In != 0 {
		return 1
	}

	return 0
}

// peerDisconnected is a zero-wait probe of a session socket. SHM sessions keep
// the socket open for their whole life and the kernel closes it when the peer
// exits, so HUP/ERR/EOF here is the dead-peer signal for both directions.
func peerDisconnected(fd int) bool {
	const maxInt32 = 1<<31 - 1
	if fd < 0 || fd > maxInt32 {
		return true
	}
	pfd := syspoll.PollFd{
		Fd:     int32(fd), // #nosec G115 -- fd is checked against int32 range above.
		Events: syspoll.In | syspoll.Hup | syspoll.Err | syspoll.RdHup,
	}

	var n int
	var errno syscall.Errno
	for {
		n, errno = syspoll.Poll(&pfd, 0)
		if n >= 0 || errno != syscall.EINTR {
			break
		}
	}
	if n < 0 {
		return true
	}
	if n == 0 {
		return false
	}
	if pfd.Revents&(syspoll.Hup|syspoll.Err|syspoll.Nval|syspoll.RdHup) != 0 {
		return true
	}
	if pfd.Revents&syspoll.In != 0 {
		var b [1]byte
		for {
			m, _, err := syscall.Recvfrom(fd, b[:], syscall.MSG_PEEK|syscall.MSG_DONTWAIT)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				return err != syscall.EAGAIN && err != syscall.EWOULDBLOCK
			}
			return m == 0
		}
	}
	return false
}
