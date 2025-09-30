// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type ssacliBinary interface {
	controllersInfo() ([]byte, error)
}

func newSsacliExec(timeout time.Duration, log *logger.Logger) *ssacliExec {
	return &ssacliExec{
		Logger:  log,
		timeout: timeout,
	}
}

type ssacliExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *ssacliExec) controllersInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "ssacli-controllers-info")
}
