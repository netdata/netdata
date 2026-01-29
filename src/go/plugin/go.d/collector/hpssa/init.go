// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func (c *Collector) initSsacliBinary() (ssacliBinary, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectSsacliExec()
	}
	return c.initNdsudoSsacliExec()
}

func (c *Collector) initNdsudoSsacliExec() (ssacliBinary, error) {
	ssacliExec := newNdsudoSsacliExec(c.Timeout.Duration(), c.Logger)
	return ssacliExec, nil
}

func (c *Collector) initDirectSsacliExec() (ssacliBinary, error) {
	// Try to find ssacli in PATH first
	for _, name := range []string{"ssacli", "SSACLI", "hpssacli"} {
		if path, err := exec.LookPath(name); err == nil {
			c.Debugf("found ssacli at: %s", path)
			return newDirectSsacliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	// Try default Windows installation paths
	defaultPaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Smart Storage Administrator", "ssacli", "bin", "ssacli.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "HP", "hpssacli", "bin", "hpssacli.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Compaq", "Hpacucli", "Bin", "hpacucli.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Smart Storage Administrator", "ssacli", "bin", "ssacli.exe"),
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			c.Debugf("found ssacli at: %s", path)
			return newDirectSsacliExec(path, c.Timeout.Duration(), c.Logger), nil
		}
	}

	return nil, fmt.Errorf("ssacli executable not found in PATH or default locations")
}
