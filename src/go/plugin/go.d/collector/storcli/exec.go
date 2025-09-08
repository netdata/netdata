// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
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
	return cmd.RunNDSudo(e.Logger, e.timeout, "storcli-controllers-info")
}

func (e *storCliExec) drivesInfo() ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "storcli-drives-info")
}
