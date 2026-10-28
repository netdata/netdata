// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package receiver

import (
	"os"
	"syscall"
)

func openEngineStateFile(path string) (*os.File, error) {
	// Nonblocking prevents a raced-in FIFO from waiting for a writer before fstat.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
