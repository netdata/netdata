// SPDX-License-Identifier: GPL-3.0-or-later

// Package credentialfile reads explicitly configured credential files. On Unix,
// each Reader owns a lazy, persistent nd-run process with reduced authority.
// Windows retains reads under the service account.
package credentialfile

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// RegularReader is the dependency needed by bounded credential-file consumers.
type RegularReader interface {
	Read(context.Context, string) ([]byte, error)
}

// FileReader is an explicitly owned credential-file dependency. Its owner MUST
// close it when the collector or discovery operation ends.
type FileReader interface {
	RegularReader
	ReadAll(context.Context, string) ([]byte, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Stat(context.Context, string) (time.Time, error)
	Close() error
}

var _ FileReader = (*Reader)(nil)

// Read performs one bounded regular-file read and closes its reader.
func Read(ctx context.Context, path string) ([]byte, error) {
	r := New()
	defer r.Close()
	return r.Read(ctx, path)
}

// ReadAll performs one unbounded read and closes its reader.
func ReadAll(ctx context.Context, path string) ([]byte, error) {
	r := New()
	defer r.Close()
	return r.ReadAll(ctx, path)
}

type fileError struct {
	op, path string
	cause    error
}

func (e *fileError) Error() string        { return fmt.Sprintf("%s %q: %v", e.op, e.path, e.cause) }
func (e *fileError) Unwrap() error        { return e.cause }
func (e *fileError) Is(target error) bool { return target == safefile.ErrFile }

// Transport errors deliberately do not unwrap OS errors: failure to start the
// helper is not evidence that the requested credential file is missing.
func transportError(operation string) error {
	return fmt.Errorf("%w: credential reader %s", safefile.ErrFile, operation)
}

func startError(err error) error {
	// Keep the launch cause useful to operators without classifying it as a file
	// response. Exec errors contain the executable path, not credential contents.
	return fmt.Errorf("%w: credential reader start failed: %v", safefile.ErrFile, err)
}
