// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type smbStatusBinary interface {
	profile() ([]byte, error)
}

func newSmbStatusBinary(timeout time.Duration, log *logger.Logger) smbStatusBinary {
	return &smbStatusExec{
		Logger:  log,
		timeout: timeout,
	}
}

type smbStatusExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *smbStatusExec) profile() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "smbstatus-profile")
}
