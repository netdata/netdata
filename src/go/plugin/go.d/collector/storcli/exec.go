// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type storCli interface {
	controllersInfo() ([]byte, error)
	drivesInfo() ([]byte, error)
}

func newStorCliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *storCliExec {
	return &storCliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type storCliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *storCliExec) controllersInfo() ([]byte, error) {
	return e.execute("storcli-controllers-info")
}

func (e *storCliExec) drivesInfo() ([]byte, error) {
	return e.execute("storcli-drives-info")
}

func (e *storCliExec) execute(args ...string) ([]byte, error) {
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
