// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type ssacliBinary interface {
	controllersInfo() ([]byte, error)
}

func newSsacliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *ssacliExec {
	return &ssacliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type ssacliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *ssacliExec) controllersInfo() ([]byte, error) {
	return e.execute("ssacli-controllers-info")
}

func (e *ssacliExec) execute(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, args...)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
