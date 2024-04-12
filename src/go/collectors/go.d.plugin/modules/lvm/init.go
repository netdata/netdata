// SPDX-License-Identifier: GPL-3.0-or-later

package lvm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/go.d.plugin/agent/executable"
)

func (l *LVM) initLVMCLIExec() (lvmCLI, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	lvmExec := newLVMCLIExec(ndsudoPath, l.Timeout.Duration(), l.Logger)

	return lvmExec, nil
}
