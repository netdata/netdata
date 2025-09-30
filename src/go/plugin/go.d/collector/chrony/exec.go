// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type chronyBinary interface {
	serverStats() ([]byte, error)
}

func newChronycExec(timeout time.Duration, log *logger.Logger) *chronycExec {
	return &chronycExec{
		Logger:  log,
		timeout: timeout,
	}
}

type chronycExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *chronycExec) serverStats() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "chronyc-serverstats")
}
