// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"os"
	"os/exec"
	"strings"
)

func (s *Sensors) initSensorsBinary() (sensorsBinary, error) {
	if s.BinaryPath == "" {
		return nil, nil
	}

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
