// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package safefile

import (
	"os"
	"path/filepath"
)

func repairDirectoryMode(path string, perm os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return &os.PathError{
			Op:   "chmod directory",
			Path: path,
			Err:  os.ErrInvalid,
		}
	}
	// Ancestry is trusted and may contain stable symlinks. The mutable leaf is not.
	parentPath, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	parent, err := openDir(parentPath, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	// Unlike path-based Chmod, the syscall refuses to follow a raced-in leaf link.
	// It can act on a replacement leaf itself; no ownership inference is made.
	if err := chmodDirectoryAt(int(parent.Fd()), filepath.Base(path), perm); err != nil {
		return &os.PathError{
			Op:   "chmod directory",
			Path: path,
			Err:  err,
		}
	}
	return nil
}
