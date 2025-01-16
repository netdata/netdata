// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type ethtoolCli interface {
	moduleEeprom(iface string) ([]byte, error)
}

func newEthtoolExec(binPath string, timeout time.Duration) *ethtoolCLIExec {
	return &ethtoolCLIExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type ethtoolCLIExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *ethtoolCLIExec) moduleEeprom(iface string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "-m", iface)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		out := strings.ReplaceAll(string(bs), "\n", " ")
		return nil, fmt.Errorf("error on '%s': %v (%s)", cmd, err, out)
	}

	return bs, nil
}
