// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix && !windows

package receiver

import (
	"errors"
	"os"
)

func openEngineStateFile(path string) (*os.File, error) {
	return nil, &os.PathError{Op: "open without following symlinks", Path: path, Err: errors.ErrUnsupported}
}
