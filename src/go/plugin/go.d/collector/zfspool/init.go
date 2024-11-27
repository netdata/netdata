// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (c *Collector) validateConfig() error {
	if c.BinaryPath == "" {
		return errors.New("no zpool binary path specified")
	}
	return nil
}

func (c *Collector) initZPoolCLIExec() (zpoolCli, error) {
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

	zpoolExec := newZpoolCLIExec(binPath, c.Timeout.Duration())
	zpoolExec.Logger = c.Logger

	return zpoolExec, nil
}
