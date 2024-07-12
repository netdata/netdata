// SPDX-License-Identifier: GPL-3.0-or-later

package ap

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newIWDevExec(binPath string, timeout time.Duration) *iwDevExec {
	return &iwDevExec{
		binPath: binPath,
		timeout: timeout,
	}
}

func newIWStationDumpExec(binPath string, timeout time.Duration) *iwStationDumpExec {
	return &iwStationDumpExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type iwDevExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

type iwStationDumpExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (a *iwDevExec) list() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.binPath, "dev")
	a.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func (a *iwStationDumpExec) list(ifaceName string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.binPath, ifaceName, "station", "dump")
	a.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
