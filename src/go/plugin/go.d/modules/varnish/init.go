// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (v *Varnish) validateConfig() error {
	if v.BinaryPath == "" {
		return errors.New("no iw binary path specified")
	}
	return nil
}

func (v *Varnish) initIwExec() (varnishStatBinary, error) {
	binPath := v.BinaryPath

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

	vs := newVarnishStatExec(binPath, v.Timeout.Duration())

	return vs, nil
}
