// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix && !linux

package safefile

import (
	"os"

	"golang.org/x/sys/unix"
)

func chmodDirectoryAt(parent int, name string, perm os.FileMode) error {
	return unix.Fchmodat(parent, name, uint32(perm.Perm()), unix.AT_SYMLINK_NOFOLLOW)
}
