// SPDX-License-Identifier: GPL-3.0-or-later

// Package credentialfile reads explicitly configured credential files. On Unix,
// each operation owns one nd-run process with reduced authority.
// Windows retains reads under the service account.
package credentialfile

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

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
