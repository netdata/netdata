// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ap

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type iwBinary interface {
	devices() ([]byte, error)
	stationStatistics(ifaceName string) ([]byte, error)
}

func newIwExec(binPath string, timeout time.Duration) *iwCliExec {
	return &iwCliExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type iwCliExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *iwCliExec) devices() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "dev")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func (e *iwCliExec) stationStatistics(ifaceName string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, ifaceName, "station", "dump")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
