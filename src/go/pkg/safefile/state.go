// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"crypto/rand"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
)

// CheckRegular rejects an existing non-regular leaf, including a dangling symlink.
// It is a preflight check, not a replacement for no-follow open and descriptor validation.
func CheckRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return &os.PathError{
			Op:   "lstat",
			Path: path,
			Err:  ErrNotRegular,
		}
	}
	return nil
}

func openNoFollow(path string) (*os.File, error) {
	flags, err := NoFollowFlags()
	if err != nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: path,
			Err:  err,
		}
	}
	return os.OpenFile(path, os.O_RDONLY|flags, 0)
}

// ReadNoFollow reads a regular state file bounded by limit, refusing symlinks at
// the file and immediate parent directory. Missing files retain ErrNotExist.
// Directory ancestry must be trusted and stable throughout the operation.
func ReadNoFollow(path string, limit int64) (data []byte, err error) {
	if limit < 0 || limit == math.MaxInt64 {
		return nil, newFileError("read", path, os.ErrInvalid)
	}
	if err := checkDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if err := CheckRegular(path); err != nil {
		return nil, err
	}
	file, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			data, err = nil, closeErr
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, newFileError("read", path, ErrNotRegular)
	}
	if info.Size() > limit {
		return nil, newFileError("read", path, ErrTooLarge)
	}
	data, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, newFileError("read", path, err)
	}
	if int64(len(data)) > limit {
		return nil, newFileError("read", path, ErrTooLarge)
	}
	return data, nil
}

// Mode controls creation permissions. Exact applies Mode through the verified
// descriptor, overriding umask; otherwise creation respects umask and existing
// directories keep their permissions.
type Mode struct {
	Perm  os.FileMode
	Exact bool
}

// Replace writes, syncs and closes a fresh exclusive same-directory temporary file,
// then renames it over path. It rejects existing non-regular destination leaves.
// Errors mean rename did not succeed. On success the replacement is visible, but
// the caller MUST sync the parent directory before claiming crash durability.
// Cleanup touches only this call's temporary file; crash orphans are left alone.
// As with os.Rename, atomic replacement is not guaranteed on non-Unix platforms.
// Parent directory ancestry must be trusted and stable throughout the operation.
func Replace(path string, data []byte, mode Mode) error {
	return replace(path, data, mode, os.Rename)
}

func replace(path string, data []byte, mode Mode, rename func(string, string) error) error {
	if err := checkDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := CheckRegular(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Random names avoid restart collisions; exclusive creation also rejects symlinks.
	tmp := path + "." + rand.Text() + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm)
	if err != nil {
		return err
	}
	closed, renamed := false, false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if !renamed {
			_ = os.Remove(tmp)
		}
	}()
	if mode.Exact {
		if err := file.Chmod(mode.Perm); err != nil {
			return err
		}
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	err = file.Close()
	closed = true
	if err != nil {
		return err
	}
	if err := rename(tmp, path); err != nil {
		return err
	}
	renamed = true
	return nil
}

func openDir(path string, writable bool) (*os.File, error) {
	path = filepath.Clean(path)
	// Reject known special objects before open, then verify the opened descriptor.
	if err := checkDir(path); err != nil {
		return nil, err
	}
	file, err := openDirectoryNoFollow(path, writable)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		err = &os.PathError{
			Op:   "open directory",
			Path: path,
			Err:  os.ErrInvalid,
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func checkDir(path string) error {
	// Named-child operations need search access, not directory-read access.
	// This metadata preflight relies on the documented stable ancestry boundary.
	path = filepath.Clean(path)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return &os.PathError{
			Op:   "inspect directory",
			Path: path,
			Err:  os.ErrInvalid,
		}
	}
	return nil
}

// EnsureDir creates missing directories and verifies the final directory without
// following its leaf symlink. Exact mode normally uses the target descriptor;
// unreadable Unix directories use a no-follow chmod relative to a verified parent.
// It does not sync directory entries; ancestry must be trusted and stable.
func EnsureDir(path string, mode Mode) error {
	path = filepath.Clean(path)
	if err := os.MkdirAll(path, mode.Perm); err != nil {
		return err
	}
	dir, err := openDir(path, mode.Exact)
	if mode.Exact && errors.Is(err, os.ErrPermission) {
		// Some owned directories can be chmodded but not opened for reading.
		// Repair without following the leaf, then verify the opened directory again.
		if err := repairDirectoryMode(path, mode.Perm); err != nil {
			return err
		}
		dir, err = openDir(path, mode.Exact)
	}
	if err != nil {
		return err
	}
	if mode.Exact {
		if err := dir.Chmod(mode.Perm); err != nil {
			_ = dir.Close()
			return err
		}
	}
	return dir.Close()
}

// SyncDir verifies and syncs a directory. Windows verifies its type but skips
// fsync, retaining the existing collector platform limitation.
func SyncDir(path string) error {
	dir, err := openDir(path, false)
	if err != nil {
		return err
	}
	var syncErr error
	if runtime.GOOS != "windows" {
		syncErr = dir.Sync()
	}
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
