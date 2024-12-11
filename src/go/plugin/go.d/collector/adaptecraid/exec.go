// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type arcconfCli interface {
	logicalDevicesInfo() ([]byte, error)
	physicalDevicesInfo() ([]byte, error)
}

func newArcconfCliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *arcconfCliExec {
	return &arcconfCliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type arcconfCliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *arcconfCliExec) logicalDevicesInfo() ([]byte, error) {
	return e.execute("arcconf-ld-info")
}

func (e *arcconfCliExec) physicalDevicesInfo() ([]byte, error) {
	return e.execute("arcconf-pd-info")
}

func (e *arcconfCliExec) execute(args ...string) ([]byte, error) {
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
