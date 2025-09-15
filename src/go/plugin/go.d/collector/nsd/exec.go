// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type nsdControlBinary interface {
	stats() ([]byte, error)
}

func newNsdControlExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *nsdControlExec {
	return &nsdControlExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type nsdControlExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *nsdControlExec) stats() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "nsd-control-stats")
}
