// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (s *Sensors) validateConfig() error {
	if s.BinaryPath == "" {
		return errors.New("no sensors binary path specified")
	}
	return nil
}

func (s *Sensors) initSensorsCliExec() (sensorsCLI, error) {
	binPath := s.BinaryPath

	if !strings.HasPrefix(binPath, "/") {
		path, err := exec.LookPath(binPath)
		if err != nil {
			return nil, err
		}
		binPath = path
	}

	if _, err := os.Stat(binPath); err != nil {
		return nil, err
	}

	sensorsExec := newSensorsCliExec(binPath, s.Timeout.Duration())
	sensorsExec.Logger = s.Logger

	return sensorsExec, nil
}
