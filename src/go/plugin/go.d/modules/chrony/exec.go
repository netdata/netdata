// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type chronyBinary interface {
	serverStats() ([]byte, error)
}

func newChronycExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *chronycExec {
	return &chronycExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type chronycExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *chronycExec) serverStats() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, "chronyc-serverstats")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
