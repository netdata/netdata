// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package megacli

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type megaCli interface {
	physDrivesInfo() ([]byte, error)
	bbuInfo() ([]byte, error)
}

func newMegaCliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *megaCliExec {
	return &megaCliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type megaCliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *megaCliExec) physDrivesInfo() ([]byte, error) {
	return e.execute("megacli-disk-info")
}

func (e *megaCliExec) bbuInfo() ([]byte, error) {
	return e.execute("megacli-battery-info")
}

func (e *megaCliExec) execute(args ...string) ([]byte, error) {
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
