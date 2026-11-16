// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package safefile

import "os"

func repairDirectoryMode(_ string, _ os.FileMode) error { return os.ErrPermission }
