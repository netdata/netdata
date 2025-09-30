// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type dmsetupCli interface {
	cacheStatus() ([]byte, error)
}

func newDmsetupExec(timeout time.Duration, log *logger.Logger) *dmsetupExec {
	return &dmsetupExec{
		Logger:  log,
		timeout: timeout,
	}
}

type dmsetupExec struct {
	*logger.Logger

	timeout time.Duration
}

func (e *dmsetupExec) cacheStatus() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "dmsetup-status-cache")
}
