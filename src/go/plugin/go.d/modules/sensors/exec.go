// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newSensorsCliExec(binPath string, timeout time.Duration) *sensorsCliExec {
	return &sensorsCliExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type sensorsCliExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *sensorsCliExec) sensorsInfo() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "-A", "-u")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
