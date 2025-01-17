// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type ethtoolCli interface {
	moduleEeprom(iface string) ([]byte, error)
}

func newEthtoolExec(ndsudoPath string, timeout time.Duration, logger *logger.Logger) *ethtoolCLIExec {
	return &ethtoolCLIExec{
		Logger:     logger,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type ethtoolCLIExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *ethtoolCLIExec) moduleEeprom(iface string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		e.ndsudoPath,
		"ethtool-module-info",
		"--devname",
		iface,
	)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
