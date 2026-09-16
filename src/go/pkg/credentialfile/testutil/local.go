// SPDX-License-Identifier: GPL-3.0-or-later

// Package testutil provides an explicitly local reader for unit tests.
// Production callers must use credentialfile's authority boundary.
package testutil

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

type localReader struct{}

func New() *localReader {
	return &localReader{}
}

func (*localReader) Read(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return safefile.Read(path)
}
func (*localReader) ReadAll(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
func (*localReader) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Open(path)
}
func (*localReader) Stat(ctx context.Context, path string) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	s, e := os.Stat(path)
	if e != nil {
		return time.Time{}, e
	}
	return s.ModTime(), nil
}
