// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (c *Collector) validateConfig() error {
	if c.OpticInterfaces == "" {
		return errors.New("no optic interfaces specified")
	}
	if c.BinaryPath == "" {
		return errors.New("no ethtool binary path specified")
	}
	return nil
}

func (c *Collector) initEthtoolCli() (ethtoolCli, error) {
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

	et := newEthtoolExec(binPath, c.Timeout.Duration())
	et.Logger = c.Logger

	return et, nil
}
