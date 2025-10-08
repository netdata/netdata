// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type eximBinary interface {
	countMessagesInQueue() ([]byte, error)
}

func newEximExec(timeout time.Duration, log *logger.Logger) *eximExec {
	return &eximExec{
		Logger:  log,
		timeout: timeout,
	}
}

type eximExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *eximExec) countMessagesInQueue() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "exim-bpc")
}
