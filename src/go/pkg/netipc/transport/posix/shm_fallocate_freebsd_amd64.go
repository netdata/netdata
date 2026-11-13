//go:build freebsd && amd64

package posix

import "syscall"

// posixFallocate returns the error number: the syscall reports it in the
// return value, not in errno.
func posixFallocate(fd int, off, length int64) syscall.Errno {
	r1, _, errno := syscall.Syscall(syscall.SYS_POSIX_FALLOCATE,
		uintptr(fd), uintptr(off), uintptr(length))
	if errno != 0 {
		return errno
	}
	return syscall.Errno(r1)
}
