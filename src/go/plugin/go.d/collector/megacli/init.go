// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func (c *Collector) initMegaCliExec() (megaCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectMegaCliExec()
	}
	return c.initNdsudoMegaCliExec()
}

func (c *Collector) initNdsudoMegaCliExec() (megaCli, error) {
	megaExec := newNdsudoMegaCliExec(c.Timeout.Duration(), c.Logger)
	return megaExec, nil
}

func (c *Collector) initDirectMegaCliExec() (megaCli, error) {
	// Try to find megacli in PATH first (try multiple names)
	for _, name := range []string{"megacli", "MegaCli", "MegaCli64", "megacli64"} {
		if path, err := exec.LookPath(name); err == nil {
			c.Debugf("found megacli at: %s", path)
			return newDirectMegaCliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	// Try default Windows installation paths
	defaultPaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "LSI", "MegaCLI", "MegaCli64.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Broadcom", "MegaCLI", "MegaCli64.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "MegaCLI", "MegaCli64.exe"),
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			c.Debugf("found megacli at: %s", path)
			return newDirectMegaCliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	return nil, fmt.Errorf("megacli executable not found in PATH or default locations")
}
