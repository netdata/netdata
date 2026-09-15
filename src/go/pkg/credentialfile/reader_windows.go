// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package credentialfile

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// Reader retains local service-account file reads on Windows.
type Reader struct{ closed atomic.Bool }

// New creates a reader. No helper is started on Windows.
func New() *Reader { return &Reader{} }

// Close marks the reader closed. Windows local operations retain native semantics.
func (r *Reader) Close() error { r.closed.Store(true); return nil }
func (r *Reader) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.closed.Load() {
		return transportError("closed")
	}
	return nil
}

// Read reads a bounded regular file under the current service account.
func (r *Reader) Read(ctx context.Context, path string) ([]byte, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	return safefile.Read(path)
}

// ReadAll reads the entire file under the current service account.
func (r *Reader) ReadAll(ctx context.Context, path string) ([]byte, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// Open opens a stream under the current service account.
func (r *Reader) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Stat returns the file modification time under the current service account.
func (r *Reader) Stat(ctx context.Context, path string) (time.Time, error) {
	if err := r.check(ctx); err != nil {
		return time.Time{}, err
	}
	i, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return i.ModTime(), nil
}
