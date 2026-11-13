//go:build linux || freebsd

package posix

import "syscall"

// Sockets must not leak into children a consumer forks and execs: an
// inherited session socket keeps the peer from ever seeing EOF, which is the
// dead-peer signal.

func udsSocket() (int, error) {
	return syscall.Socket(syscall.AF_UNIX, sockType|syscall.SOCK_CLOEXEC, 0)
}

// prepareListener leaves the listener blocking: accept4 sets the flag
// atomically.
func prepareListener(int) error {
	return nil
}

func acceptCloexec(fd int) (int, error) {
	nfd, _, err := syscall.Accept4(fd, syscall.SOCK_CLOEXEC)
	return nfd, err
}
