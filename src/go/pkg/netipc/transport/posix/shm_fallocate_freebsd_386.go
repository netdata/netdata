//go:build freebsd && 386

package posix

import "syscall"

// posixFallocate returns the error number: the syscall reports it in the
// return value, not in errno. Each 64-bit off_t takes two stack words.
func posixFallocate(fd int, off, length int64) syscall.Errno {
	r1, _, errno := syscall.Syscall6(syscall.SYS_POSIX_FALLOCATE,
		uintptr(fd), uintptr(uint32(off)), uintptr(uint32(off>>32)),
		uintptr(uint32(length)), uintptr(uint32(length>>32)), 0)
	if errno != 0 {
		return errno
	}
	return syscall.Errno(r1)
}
