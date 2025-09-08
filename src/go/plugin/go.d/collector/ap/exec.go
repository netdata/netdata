// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ap

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type iwBinary interface {
	devices() ([]byte, error)
	stationStatistics(ifaceName string) ([]byte, error)
}

func newIwExec(binPath string, timeout time.Duration) *iwCliExec {
	return &iwCliExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type iwCliExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *iwCliExec) devices() ([]byte, error) {
	return cmd.RunUnprivileged(e.Logger, e.timeout, e.binPath, "dev")
}

func (e *iwCliExec) stationStatistics(ifaceName string) ([]byte, error) {
	return cmd.RunUnprivileged(e.Logger, e.timeout, e.binPath, ifaceName, "station", "dump")
}
