// SPDX-License-Identifier: GPL-3.0-or-later

// Package credentialfiletest provides an explicitly local reader for unit tests.
// Production callers must use credentialfile.Reader's authority boundary.
package credentialfiletest

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

type localReader struct{}

func New(t testing.TB) *localReader {
	t.Helper()
	r := &localReader{}
	t.Cleanup(func() { _ = r.Close() })
	return r
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
func (*localReader) Close() error { return nil }
