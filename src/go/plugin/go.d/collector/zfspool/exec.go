// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type zpoolCli interface {
	list() ([]byte, error)
	listWithVdev(pool string) ([]byte, error)
}

func newZpoolCLIExec(binPath string, timeout time.Duration) *zpoolCLIExec {
	return &zpoolCLIExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type zpoolCLIExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *zpoolCLIExec) list() ([]byte, error) {
	return ndexec.RunUnprivileged(e.Logger, e.timeout, e.binPath, "list", "-p")
}

func (e *zpoolCLIExec) listWithVdev(pool string) ([]byte, error) {
	return ndexec.RunUnprivileged(e.Logger, e.timeout, e.binPath, "list", "-p", "-v", "-L", pool)
}
