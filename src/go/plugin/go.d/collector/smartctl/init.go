// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package smartctl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
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
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	smartctlExec := newSmartctlCliExec(ndsudoPath, c.Timeout.Duration(), c.Logger)

	return smartctlExec, nil
}
