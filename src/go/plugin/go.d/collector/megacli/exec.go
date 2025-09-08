// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package megacli

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
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
	return cmd.RunNDSudo(e.Logger, e.timeout, "megacli-disk-info")
}

func (e *megaCliExec) bbuInfo() ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "megacli-battery-info")
}
