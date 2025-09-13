// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type smbStatusBinary interface {
	profile() ([]byte, error)
}

func newSmbStatusBinary(ndsudoPath string, timeout time.Duration, log *logger.Logger) smbStatusBinary {
	return &smbStatusExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type smbStatusExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *smbStatusExec) profile() ([]byte, error) {
	return cmd.RunNDSudo(e.Logger, e.timeout, "smbstatus-profile")
}
