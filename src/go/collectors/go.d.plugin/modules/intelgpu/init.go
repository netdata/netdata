// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"fmt"
	"os"
	"os/exec"
)

func (ig *IntelGPU) initIntelGPUTopExec() (intelGpuTop, error) {
	binPath := ig.BinaryPath
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		path, err := exec.LookPath(ig.binName)
		if err != nil {
			return nil, fmt.Errorf("error on lookup '%s': %v", ig.binName, err)
		}
		binPath = path
	}

	return newIntelGpuTopExec(binPath, ig.UpdateEvery)
}
