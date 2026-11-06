//go:build unix && !linux && !freebsd

package posix

import "syscall"

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

// accept may block, so it cannot hold ForkLock; the flag follows at once.
func acceptCloexec(fd int) (int, error) {
	nfd, _, err := syscall.Accept(fd)
	if err == nil {
		syscall.CloseOnExec(nfd)
		setNoSigpipe(nfd)
	}
	return nfd, err
}
