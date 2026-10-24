// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package credentialfile

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// Read reads a bounded regular file under the current service account.
func Read(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return safefile.Read(path)
}

// ReadAll reads the entire file under the current service account.
func ReadAll(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// Open opens a stream under the current service account.
func Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Stat returns the file modification time under the current service account.
func Stat(ctx context.Context, path string) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	i, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return i.ModTime(), nil
}
