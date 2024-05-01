// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/go.d.plugin/agent/executable"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
)

func (s *Smartctl) validateConfig() error {
	switch s.NoCheckPowerMode {
	case "never", "sleep", "standby", "idle":
	default:
		return fmt.Errorf("invalid power mode '%s'", s.NoCheckPowerMode)
	}
	return nil
}

func (s *Smartctl) initDeviceSelector() (matcher.Matcher, error) {
	if s.DeviceSelector == "" {
		return matcher.TRUE(), nil
	}

	m, err := matcher.NewSimplePatternsMatcher(s.DeviceSelector)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (s *Smartctl) initSmartctlCli() (smartctlCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	smartctlExec := newSmartctlCliExec(ndsudoPath, s.Timeout.Duration(), s.Logger)

	return smartctlExec, nil
}
