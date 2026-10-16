// SPDX-License-Identifier: GPL-3.0-or-later

// Package safefile opens configured regular files and provides bounded reads.
package safefile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxSize is the maximum number of bytes Read accepts.
const MaxSize int64 = 1 << 20

var (
	// ErrFile identifies every failure returned by Open and Read.
	ErrFile = errors.New("safe file read failed")
	// ErrNotRegular identifies a path whose opened object is not a regular file.
	ErrNotRegular = errors.New("file is not regular")
	// ErrTooLarge identifies a regular file larger than MaxSize.
	ErrTooLarge = errors.New("file exceeds size limit")
)

type fileError struct {
	op   string
	path string
	err  error
}

func (e *fileError) Error() string {
	return fmt.Sprintf("%s %q: %v", e.op, e.path, e.err)
}

func (e *fileError) Unwrap() error {
	return e.err
}

func (e *fileError) Is(target error) bool {
	return target == ErrFile || errors.Is(e.err, target)
}

// Open follows symlinks and opens a configured regular file for reading.
// Unix opens are nonblocking, so FIFOs are rejected without waiting for a writer.
// It imposes no size limit; the caller owns both read bounds and closing the file.
func Open(path string) (*os.File, error) {
	file, _, err := openRegular(path)
	return file, err
}

func openRegular(path string) (*os.File, os.FileInfo, error) {
	file, err := open(path)
	if err != nil {
		return nil, nil, newFileError("open", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, newFileError("stat", path, err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, newFileError("read", path, ErrNotRegular)
	}
	return file, info, nil
}

// Read returns the contents of a configured regular file no larger than
// MaxSize. The opened object is checked so a path replacement cannot bypass
// the object-type validation.
func Read(path string) (data []byte, err error) {
	file, info, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			data = nil
			err = newFileError("close", path, closeErr)
		}
	}()
	if info.Size() > MaxSize {
		return nil, newFileError("read", path, ErrTooLarge)
	}
	data, err = readBounded(file, info.Size())
	if err != nil {
		return nil, newFileError("read", path, err)
	}
	return data, nil
}

func readBounded(r io.Reader, size int64) ([]byte, error) {
	if size < 0 {
		size = 0
	}
	if size > MaxSize {
		size = MaxSize
	}

	buf := bytes.NewBuffer(make([]byte, 0, int(size)+bytes.MinRead))
	n, err := buf.ReadFrom(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return nil, err
	}
	if n > MaxSize {
		return nil, ErrTooLarge
	}
	return buf.Bytes(), nil
}

func newFileError(op, path string, err error) error {
	return &fileError{op: op, path: path, err: err}
}
