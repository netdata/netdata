// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (ig *IntelGPU) initIntelGPUTopExec() (intelGpuTop, error) {
	ndsudoPath := filepath.Join(executable.Directory, ig.ndsudoName)
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	return newIntelGpuTopExec(ig.Logger, ndsudoPath, ig.UpdateEvery, ig.Device)
}
