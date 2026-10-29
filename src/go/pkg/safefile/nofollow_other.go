// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix && !windows

package safefile

import (
	"errors"
	"os"
)

// NoFollowFlags fails closed on platforms without a no-follow implementation.
func NoFollowFlags() (int, error) { return 0, errors.ErrUnsupported }

func openDirectoryNoFollow(path string, _ bool) (*os.File, error) { return openNoFollow(path) }
