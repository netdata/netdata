// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type nsdControlBinary interface {
	stats() ([]byte, error)
}

func newNsdControlExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *nsdControlExec {
	return &nsdControlExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type nsdControlExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *nsdControlExec) stats() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, "nsd-control-stats")

	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
