// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type ssacliBinary interface {
	controllersInfo() ([]byte, error)
}

// ndsudoSsacliExec executes ssacli via ndsudo (Linux/BSD)
type ndsudoSsacliExec struct {
	*logger.Logger

	timeout time.Duration
}

func newNdsudoSsacliExec(timeout time.Duration, log *logger.Logger) *ndsudoSsacliExec {
	return &ndsudoSsacliExec{
		Logger:  log,
		timeout: timeout,
	}
}

func (e *ndsudoSsacliExec) controllersInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "ssacli-controllers-info")
}

// directSsacliExec executes ssacli directly (Windows)
type directSsacliExec struct {
	*logger.Logger

	ssacliPath string
	timeout    time.Duration
}

func newDirectSsacliExec(ssacliPath string, timeout time.Duration, log *logger.Logger) *directSsacliExec {
	return &directSsacliExec{
		Logger:     log,
		ssacliPath: ssacliPath,
		timeout:    timeout,
	}
}

func (e *directSsacliExec) controllersInfo() ([]byte, error) {
	return e.execute("ctrl", "all", "show", "config", "detail")
}

func (e *directSsacliExec) execute(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ssacliPath, args...)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("'%s' execution failed: %v", cmd, err)
	}

	return bs, nil
}
