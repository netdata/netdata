// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"os"
	"syscall"
)

// NoFollowFlags opens the reparse point itself so descriptor validation rejects
// it without opening its target. Go 1.27 passes this flag to CreateFile.
// Callers MUST check the opened descriptor's type before using it.
func NoFollowFlags() (int, error) {
	return syscall.FILE_FLAG_OPEN_REPARSE_POINT, nil
}

func openDirectoryNoFollow(path string, writable bool) (*os.File, error) {
	if !writable {
		return openNoFollow(path)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{
			Op:   "open directory",
			Path: path,
			Err:  err,
		}
	}
	// Chmod of a read-only directory needs FILE_WRITE_ATTRIBUTES on its handle.
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.FILE_WRITE_ATTRIBUTES,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &os.PathError{
			Op:   "open directory",
			Path: path,
			Err:  err,
		}
	}
	return os.NewFile(uintptr(handle), path), nil
}
