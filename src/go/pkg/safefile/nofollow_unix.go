// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package safefile

import (
	"os"
	"syscall"
)

// NoFollowFlags returns flags for callers whose library owns the open. These
// refuse leaf symlinks and prevent FIFO opens from waiting on Unix. Callers MUST
// still verify the opened descriptor's type before reading or exposing ownership.
func NoFollowFlags() (int, error) {
	return syscall.O_NOFOLLOW | syscall.O_NONBLOCK, nil
}

func openDirectoryNoFollow(path string, _ bool) (*os.File, error) { return openNoFollow(path) }
