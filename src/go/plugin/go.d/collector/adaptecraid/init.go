// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func (c *Collector) initArcconfCliExec() (arcconfCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectArcconfCliExec()
	}
	return c.initNdsudoArcconfCliExec()
}

func (c *Collector) initNdsudoArcconfCliExec() (arcconfCli, error) {
	arcconfExec := newNdsudoArcconfCliExec(c.Timeout.Duration(), c.Logger)
	return arcconfExec, nil
}

func (c *Collector) initDirectArcconfCliExec() (arcconfCli, error) {
	// Try to find arcconf in PATH first
	for _, name := range []string{"arcconf", "ARCCONF"} {
		if path, err := exec.LookPath(name); err == nil {
			c.Debugf("found arcconf at: %s", path)
			return newDirectArcconfCliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	// Try default Windows installation paths
	defaultPaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Adaptec", "ARCCONF", "arcconf.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsemi", "ARCCONF", "arcconf.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Adaptec", "ARCCONF", "arcconf.exe"),
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			c.Debugf("found arcconf at: %s", path)
			return newDirectArcconfCliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	return nil, fmt.Errorf("arcconf executable not found in PATH or default locations")
}
