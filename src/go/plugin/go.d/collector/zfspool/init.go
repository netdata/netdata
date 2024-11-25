// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (z *ZFSPool) validateConfig() error {
	if z.BinaryPath == "" {
		return errors.New("no zpool binary path specified")
	}
	return nil
}

func (z *ZFSPool) initZPoolCLIExec() (zpoolCLI, error) {
	binPath := z.BinaryPath

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

	zpoolExec := newZpoolCLIExec(binPath, z.Timeout.Duration())
	zpoolExec.Logger = z.Logger

	return zpoolExec, nil
}
