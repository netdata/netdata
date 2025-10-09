// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package megacli

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type megaCli interface {
	physDrivesInfo() ([]byte, error)
	bbuInfo() ([]byte, error)
}

func newMegaCliExec(timeout time.Duration, log *logger.Logger) *megaCliExec {
	return &megaCliExec{
		Logger:  log,
		timeout: timeout,
	}
}

type megaCliExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *megaCliExec) physDrivesInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "megacli-disk-info")
}

func (e *megaCliExec) bbuInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "megacli-battery-info")
}
