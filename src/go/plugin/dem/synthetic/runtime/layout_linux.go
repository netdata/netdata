// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func renameDirectoryExclusive(parent, from, to string) error {
	dir, err := os.Open(parent)
	if err != nil {
		return err
	}
	err = unix.Renameat2(int(dir.Fd()), from, int(dir.Fd()), to, unix.RENAME_NOREPLACE)
	return errors.Join(err, dir.Close())
}
