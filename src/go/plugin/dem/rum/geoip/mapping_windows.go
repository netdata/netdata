// SPDX-License-Identifier: GPL-3.0-or-later

package geoip

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openFile(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: path,
			Err:  err,
		}
	}
	return os.NewFile(uintptr(handle), path), nil
}

func mapFile(f *os.File, size int) (*mapping, error) {
	handle, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	address, err := windows.MapViewOfFile(handle, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &mapping{
		data:    unsafe.Slice((*byte)(unsafe.Pointer(address)), size),
		release: func() error { return errors.Join(windows.UnmapViewOfFile(address), windows.CloseHandle(handle)) },
	}, nil
}
