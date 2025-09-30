// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	switch c.NoCheckPowerMode {
	case "never", "sleep", "standby", "idle":
	default:
		return fmt.Errorf("invalid power mode '%s'", c.NoCheckPowerMode)
	}

	for _, v := range c.ExtraDevices {
		if v.Name == "" || v.Type == "" {
			return fmt.Errorf("invalid extra device: name and type must both be provided, got name='%s' type='%s'", v.Name, v.Type)
		}
	}

	return nil
}

func (c *Collector) initDeviceSelector() (matcher.Matcher, error) {
	if c.DeviceSelector == "" {
		return matcher.TRUE(), nil
	}

	m, err := matcher.NewSimplePatternsMatcher(c.DeviceSelector)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (c *Collector) initSmartctlCli() (smartctlCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectSmartctlCli()
	}
	return c.initNdsudoSmartctlCli()
}

func (c *Collector) initNdsudoSmartctlCli() (smartctlCli, error) {
	smartctlExec := newNdsudoSmartctlCli(c.Timeout.Duration(), c.Logger)
	return smartctlExec, nil
}

func (c *Collector) initDirectSmartctlCli() (smartctlCli, error) {
	smartctlPath, err := exec.LookPath("smartctl")
	if err != nil {
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("smartctl executable not found in PATH: %v", err)
		}
		defaultWinPath := filepath.Join("C:\\Program Files\\smartmontools\\bin", "smartctl.exe")
		if _, err := os.Stat(defaultWinPath); err != nil {
			return nil, fmt.Errorf("smartctl executable not found in PATH or default location: %v", err)
		}
		smartctlPath = defaultWinPath
	}

	c.Debugf("found smartctl at: %s", smartctlPath)
	smartctlExec := newDirectSmartctlCli(smartctlPath, c.Timeout.Duration(), c.Logger)
	return smartctlExec, nil
}
