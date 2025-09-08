// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type chronyBinary interface {
	serverStats() ([]byte, error)
}

func newChronycExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *chronycExec {
	return &chronycExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type chronycExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *chronycExec) serverStats() ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "chronyc-serverstats")
}
