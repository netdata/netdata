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

func privateDirectory(path string) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0777 != 0700 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("runtime store must be a private directory owned by the service account")
	}
	return nil
}

func privatePreloadDirectory(path string) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0077 != 0 || st.Mode&0500 != 0500 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("preload must be a private directory owned by the service account")
	}
	return nil
}
