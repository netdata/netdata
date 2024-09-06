// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newVarnishStatExec(binPath string, timeout time.Duration) *varnishStatExec {
	return &varnishStatExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type varnishStatExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *varnishStatExec) varnishStatistics() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "-1", "-t", "1")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
