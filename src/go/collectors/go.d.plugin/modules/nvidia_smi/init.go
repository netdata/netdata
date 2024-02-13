// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"fmt"
	"os"
	"os/exec"
)

func (nv *NvidiaSMI) initNvidiaSMIExec() (nvidiaSMI, error) {
	binPath := nv.BinaryPath
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		path, err := exec.LookPath(nv.binName)
		if err != nil {
			return nil, fmt.Errorf("error on lookup '%s': %v", nv.binName, err)
		}
		binPath = path
	}

	return newNvidiaSMIExec(binPath, nv.Config, nv.Logger)
}
