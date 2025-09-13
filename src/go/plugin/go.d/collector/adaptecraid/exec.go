// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
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
	return cmd.RunNDSudo(e.Logger, e.timeout, "arcconf-ld-info")
}

func (e *arcconfCliExec) physicalDevicesInfo() ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "arcconf-pd-info")
}
