// SPDX-License-Identifier: GPL-3.0-or-later

// Package safefile reads small configured files through a bounded,
// regular-file-only path.
package safefile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// MaxSize is the maximum number of bytes Read accepts.
const MaxSize int64 = 1 << 20

var (
	// ErrFile identifies every failure returned by Read.
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

// Read returns the contents of a configured regular file no larger than
// MaxSize. The opened object is checked so a path replacement cannot bypass
// the object-type validation.
func Read(path string) (data []byte, err error) {
	file, err := open(path)
	if err != nil {
		return nil, newFileError("open", path, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			data = nil
			err = newFileError("close", path, closeErr)
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, newFileError("stat", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, newFileError("read", path, ErrNotRegular)
	}
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
