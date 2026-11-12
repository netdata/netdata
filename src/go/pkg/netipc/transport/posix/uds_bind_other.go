//go:build unix && !darwin

package posix

import "syscall"

// bindOwnerOnly binds fd to path as owner-only (0600, narrowed by the umask).
// Linux and FreeBSD give the socket file the mode of the unbound socket, so
// fchmod() before bind() sets it without a post-bind window and without
// touching the process-global umask. connect() needs write permission on the
// socket file, so a umask without owner write fails the listen instead of
// leaving an endpoint its own user cannot reach.
func bindOwnerOnly(fd int, _, _ string, path string) error {
	if err := syscall.Fchmod(fd, 0o600); err != nil {
		return wrapErr(ErrSocket, "fchmod: "+err.Error())
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		return wrapErr(ErrSocket, "bind: "+err.Error())
	}
	var st syscall.Stat_t
	err := syscall.Lstat(path, &st)
	if err == nil {
		if st.Mode&syscall.S_IWUSR != 0 {
			return nil
		}
		err = syscall.EACCES
	}
	_ = syscall.Unlink(path)
	return wrapErr(ErrSocket, "socket mode: "+err.Error())
}
