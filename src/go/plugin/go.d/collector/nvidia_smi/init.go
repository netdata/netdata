// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pathvalidate"
)

func (c *Collector) initNvidiaSmiExec() (nvidiaSmiBinary, error) {
	binPath := c.BinaryPath
	if binPath == "" || !fileExists(binPath) {
		path, err := exec.LookPath(c.binName)
		if err != nil {
			if runtime.GOOS == "windows" {
				path, err = ndexec.FindBinary(
					nil,
					[]string{
						filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
						filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe"),
					},
				)
			}
			if err != nil {
				return nil, fmt.Errorf("error on lookup '%s': %v", c.binName, err)
			}
		}
		binPath = path
	}

	validatedPath, err := pathvalidate.ValidateBinaryPath(binPath)
	if err != nil {
		return nil, err
	}

	return newNvidiaSmiBinary(validatedPath, c.Config, c.Logger)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
