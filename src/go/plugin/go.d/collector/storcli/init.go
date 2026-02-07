// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func (c *Collector) initStorCliExec() (storCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectStorCliExec()
	}
	return c.initNdsudoStorCliExec()
}

func (c *Collector) initNdsudoStorCliExec() (storCli, error) {
	storExec := newNdsudoStorCliExec(c.Timeout.Duration(), c.Logger)
	return storExec, nil
}

func (c *Collector) initDirectStorCliExec() (storCli, error) {
	// Try to find storcli in PATH first (try multiple names)
	for _, name := range []string{"storcli", "storcli64", "StorCLI", "StorCLI64"} {
		if path, err := exec.LookPath(name); err == nil {
			c.Debugf("found storcli at: %s", path)
			return newDirectStorCliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	// Try default Windows installation paths
	defaultPaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Broadcom", "StorCLI", "storcli64.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "LSI", "StorCLI", "storcli64.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "StorCLI", "storcli64.exe"),
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			c.Debugf("found storcli at: %s", path)
			return newDirectStorCliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	return nil, fmt.Errorf("storcli executable not found in PATH or default locations")
}
