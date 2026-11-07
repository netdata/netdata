//go:build unix && !darwin

package posix

import "syscall"

// bindOwnerOnly binds fd to path as owner-only (0600, narrowed by the umask).
// Linux and FreeBSD give the socket file the mode of the unbound socket, so
// fchmod() before bind() sets it without a post-bind window and without
// touching the process-global umask.
func bindOwnerOnly(fd int, _, _ string, path string) error {
	if err := syscall.Fchmod(fd, 0o600); err != nil {
		return wrapErr(ErrSocket, "fchmod: "+err.Error())
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		return wrapErr(ErrSocket, "bind: "+err.Error())
	}
	return nil
}
