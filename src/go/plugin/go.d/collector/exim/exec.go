// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type eximBinary interface {
	countMessagesInQueue() ([]byte, error)
}

func newEximExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *eximExec {
	return &eximExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type eximExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *eximExec) countMessagesInQueue() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, "exim-bpc")

	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
