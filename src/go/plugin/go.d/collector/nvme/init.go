// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func (c *Collector) initNVMeCLIExec() (nvmeCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectNvmeCliExec()
	}
	return c.initNdsudoNvmeCliExec()
}

func (c *Collector) initNdsudoNvmeCliExec() (nvmeCli, error) {
	nvmeExec := &ndsudoNvmeCliExec{timeout: c.Timeout.Duration()}
	return nvmeExec, nil
}

func (c *Collector) initDirectNvmeCliExec() (nvmeCli, error) {
	// Try to find nvme in PATH first
	for _, name := range []string{"nvme", "nvme-cli"} {
		if path, err := exec.LookPath(name); err == nil {
			c.Debugf("found nvme at: %s", path)
			return &directNvmeCliExec{
				Logger:   c.Logger,
				nvmePath: path,
				timeout:  c.Timeout.Duration(),
			}, nil
		}
	}

	// Try default Windows installation paths
	defaultPaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "nvme-cli", "nvme.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "nvme-cli", "nvme.exe"),
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			c.Debugf("found nvme at: %s", path)
			return &directNvmeCliExec{
				Logger:   c.Logger,
				nvmePath: path,
				timeout:  c.Timeout.Duration(),
			}, nil
		}
	}

	return nil, fmt.Errorf("nvme executable not found in PATH or default locations")
}
