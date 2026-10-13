// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package safefile

import (
	"os"
	"syscall"
)

func open(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
