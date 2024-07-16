// SPDX-License-Identifier: GPL-3.0-or-later

package ap

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (a *AP) validateConfig() error {
	if a.BinaryPath == "" {
		return errors.New("no iw binary path specified")
	}
	return nil
}

func (a *AP) initIwExec() (iwBinary, error) {
	binPath := a.BinaryPath

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

	iw := newIwExec(binPath, a.Timeout.Duration())

	return iw, nil
}
