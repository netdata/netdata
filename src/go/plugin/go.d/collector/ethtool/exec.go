// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type ethtoolCli interface {
	moduleEeprom(iface string) ([]byte, error)
}

func newEthtoolExec(timeout time.Duration, logger *logger.Logger) *ethtoolCLIExec {
	return &ethtoolCLIExec{
		Logger:  logger,
		timeout: timeout,
	}
}

type ethtoolCLIExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *ethtoolCLIExec) moduleEeprom(iface string) ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "ethtool-module-info", "--devname", iface)
}
