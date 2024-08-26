// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type smbStatusBinary interface {
	profile() ([]byte, error)
}

func newSmbStatusBinary(ndsudoPath string, timeout time.Duration, log *logger.Logger) smbStatusBinary {
	return &smbStatusExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type smbStatusExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *smbStatusExec) profile() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, "smbstatus-profile")

	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
