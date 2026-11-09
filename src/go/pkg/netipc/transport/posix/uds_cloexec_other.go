//go:build unix && !linux && !freebsd

package posix

import (
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/syspoll"
)

// Sockets must not leak into children a consumer forks and execs: an
// inherited session socket keeps the peer from ever seeing EOF, which is the
// dead-peer signal. Without SOCK_CLOEXEC the flag is set right after creation;
// ForkLock keeps Go's own fork+exec out of that window.

func udsSocket() (int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	fd, err := syscall.Socket(syscall.AF_UNIX, sockType, 0)
	if err == nil {
		syscall.CloseOnExec(fd)
		setNoSigpipe(fd)
	}
	return fd, err
}

// prepareListener makes the listener non-blocking, so acceptCloexec can hold
// ForkLock across accept and the close-on-exec flag without blocking in it.
func prepareListener(fd int) error {
	return syscall.SetNonblock(fd, true)
}

// acceptCloexec accepts under ForkLock, as Go's net package does: a fork
// between accept and the close-on-exec flag would leak the connection into
// the child. The listener is non-blocking, so the wait for a client polls
// without the lock.
func acceptCloexec(fd int) (int, error) {
	for {
		syscall.ForkLock.RLock()
		nfd, _, err := syscall.Accept(fd)
		if err == nil {
			syscall.CloseOnExec(nfd)
		}
		syscall.ForkLock.RUnlock()

		switch {
		case err == nil:
			// A BSD socket inherits O_NONBLOCK from its listener; sessions
			// expect a blocking descriptor.
			if err := syscall.SetNonblock(nfd, false); err != nil {
				_ = syscall.Close(nfd)
				return -1, err
			}
			setNoSigpipe(nfd)
			return nfd, nil
		case err == syscall.EINTR:
			continue
		case err != syscall.EAGAIN && err != syscall.EWOULDBLOCK:
			return -1, err
		}

		pfd := syspoll.PollFd{Fd: int32(fd), Events: syspoll.In} // #nosec G115 -- fd is an open descriptor.
		if _, errno := syspoll.Poll(&pfd, -1); errno != 0 && errno != syscall.EINTR {
			return -1, errno
		}
		if pfd.Revents&syspoll.Nval != 0 {
			return -1, syscall.EBADF
		}
	}
}
