// SPDX-License-Identifier: GPL-3.0-or-later
//go:build unix

package geoip

import (
	"os"

	"golang.org/x/sys/unix"
)

func openFile(path string) (*os.File, error) {
	// A misconfigured FIFO must not block the receiver's refresh worker or shutdown.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: path,
			Err:  err,
		}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func mapFile(f *os.File, size int) (*mapping, error) {
	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return &mapping{
		data:    data,
		release: func() error { return unix.Munmap(data) },
	}, nil
}
