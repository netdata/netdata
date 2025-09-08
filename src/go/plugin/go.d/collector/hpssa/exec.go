// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type ssacliBinary interface {
	controllersInfo() ([]byte, error)
}

func newSsacliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *ssacliExec {
	return &ssacliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type ssacliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *ssacliExec) controllersInfo() ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "ssacli-controllers-info")
}
