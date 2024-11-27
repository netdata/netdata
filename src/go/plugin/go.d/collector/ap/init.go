// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ap

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (c *Collector) validateConfig() error {
	if c.BinaryPath == "" {
		return errors.New("no iw binary path specified")
	}
	return nil
}

func (c *Collector) initIwExec() (iwBinary, error) {
	binPath := c.BinaryPath

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

	iw := newIwExec(binPath, c.Timeout.Duration())

	return iw, nil
}
