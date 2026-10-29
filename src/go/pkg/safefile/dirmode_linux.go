// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"errors"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

func chmodDirectoryAt(parent int, name string, perm os.FileMode) error {
	// O_PATH obtains an identity handle without requiring read/search permission.
	fd, err := unix.Openat(parent, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	err = unix.Fchmodat(fd, "", uint32(perm.Perm()), unix.AT_EMPTY_PATH)
	if !errors.Is(err, unix.EOPNOTSUPP) && !errors.Is(err, unix.EINVAL) {
		return err
	}
	// Older kernels lack fchmodat2/AT_EMPTY_PATH. As in Go's rooted chmod,
	// /proc/self/fd addresses the held object rather than resolving the original path.
	return chmodDirectoryViaProc(file, perm)
}

func chmodDirectoryViaProc(file *os.File, perm os.FileMode) error {
	return os.Chmod("/proc/self/fd/"+strconv.FormatUint(uint64(file.Fd()), 10), perm)
}
