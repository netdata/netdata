// SPDX-License-Identifier: GPL-3.0-or-later

package zfspool

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/logger"
)

func newZpoolCLIExec(binPath string, timeout time.Duration) *zpoolCLIExec {
	return &zpoolCLIExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type zpoolCLIExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *zpoolCLIExec) list() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "list", "-p")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
