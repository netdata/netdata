// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type ethtoolCli interface {
	moduleEeprom(iface string) ([]byte, error)
}

func newEthtoolExec(ndsudoPath string, timeout time.Duration, logger *logger.Logger) *ethtoolCLIExec {
	return &ethtoolCLIExec{
		Logger:     logger,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type ethtoolCLIExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *ethtoolCLIExec) moduleEeprom(iface string) ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "ethtool-module-info", "--devname", iface)
}
