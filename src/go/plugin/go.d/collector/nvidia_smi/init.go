// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pathvalidate"
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

	validatedPath, err := pathvalidate.ValidateBinaryPath(binPath)
	if err != nil {
		return nil, err
	}

	return newNvidiaSmiBinary(validatedPath, c.Config, c.Logger)
}
