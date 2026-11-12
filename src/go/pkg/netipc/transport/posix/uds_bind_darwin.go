//go:build darwin

package posix

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// bindOwnerOnly binds fd to path as owner-only (0600, narrowed by the umask).
// macOS rejects fchmod() on an unbound socket and creates the socket file
// 0777 & ~umask. The socket is bound inside a private 0700 directory
// {serviceName}.d, narrowed to owner-only there, then renamed into place, so
// it is never reachable with a wider mode and the process-global umask is
// untouched. The caller holds the endpoint lock, so a leftover directory
// belongs to a crashed server and is removed first. fchmodat() and open()
// resolve the directory by path without following a symlink, and the owner
// check on the opened descriptor refuses a directory another account swapped
// in. After that check every step but bind() goes through the descriptor, so
// a later swap makes the listen fail instead of placing a socket someone else
// can reach.
func bindOwnerOnly(fd int, runDir, serviceName, path string) error {
	dir := filepath.Join(runDir, serviceName+".d")
	tmp := filepath.Join(dir, "s")
	if len(tmp) >= maxSunPath {
		return ErrPathTooLong
	}
	sockErr := func(op string, err error) error { return wrapErr(ErrSocket, op+": "+err.Error()) }

	// Anything but a real directory is unlinked without following it.
	if info, err := os.Lstat(dir); err == nil {
		if info.IsDir() {
			// unlink, not os.Remove: a directory at tmp is foreign content.
			if err := syscall.Unlink(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return sockErr("remove leftover socket", err)
			}
		}
		if err := os.Remove(dir); err != nil {
			return sockErr("remove leftover bind dir", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return sockErr("stat bind dir", err)
	}

	if err := os.Mkdir(dir, 0o700); err != nil {
		return sockErr("mkdir bind dir", err)
	}
	defer func() { _ = os.Remove(dir) }()
	// Fchmodat ignores the umask, so a umask without owner bits cannot make the
	// directory unopenable or unusable.
	if err := unix.Fchmodat(unix.AT_FDCWD, dir, 0o700, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return sockErr("chmod bind dir", err)
	}
	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return sockErr("open bind dir", err)
	}
	defer func() { _ = unix.Close(dfd) }()
	var st unix.Stat_t
	if err := unix.Fstat(dfd, &st); err != nil {
		return sockErr("stat bind dir", err)
	}
	if int(st.Uid) != os.Geteuid() {
		return sockErr("bind dir", unix.EPERM)
	}
	// The descriptor is the directory every later step uses; keep it 0700
	// even if another one of ours replaced the path.
	if err := unix.Fchmod(dfd, 0o700); err != nil {
		return sockErr("chmod bind dir", err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: tmp}); err != nil {
		return sockErr("bind", err)
	}
	// The bound file carries 0777 & ~umask. It is set to exactly 0600: a
	// umask without owner write would leave a socket its own user cannot
	// connect to, and only the owner can reach the private directory.
	err = unix.Fchmodat(dfd, "s", 0o600, 0)
	if err == nil {
		err = unix.Renameat(dfd, "s", unix.AT_FDCWD, path)
	}
	if err != nil {
		_ = unix.Unlinkat(dfd, "s", 0)
		return sockErr("place socket", err)
	}
	return nil
}
