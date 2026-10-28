// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"os"
	"syscall"
)

func openEngineStateFile(path string) (*os.File, error) {
	// Open the reparse point itself so the descriptor check rejects a symlink
	// without ever opening its target. Go 1.27 passes this flag to CreateFile.
	return os.OpenFile(path, os.O_RDONLY|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}
