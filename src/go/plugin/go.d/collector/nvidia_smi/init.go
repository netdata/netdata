// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"fmt"
	"os"
	"os/exec"
)

func (c *Collector) initNvidiaSmiExec() (nvidiaSmiBinary, error) {
	binPath := c.BinaryPath
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		path, err := exec.LookPath(c.binName)
		if err != nil {
			return nil, fmt.Errorf("error on lookup '%s': %v", c.binName, err)
		}
		binPath = path
	}

	return newNvidiaSmiBinary(binPath, c.Config, c.Logger)
}
