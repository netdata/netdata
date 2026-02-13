// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type storCli interface {
	controllersInfo() ([]byte, error)
	drivesInfo() ([]byte, error)
}

// ndsudoStorCliExec executes storcli via ndsudo (Linux/BSD)
type ndsudoStorCliExec struct {
	*logger.Logger

	timeout time.Duration
}

func newNdsudoStorCliExec(timeout time.Duration, log *logger.Logger) *ndsudoStorCliExec {
	return &ndsudoStorCliExec{
		Logger:  log,
		timeout: timeout,
	}
}

func (e *ndsudoStorCliExec) controllersInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "storcli-controllers-info")
}

func (e *ndsudoStorCliExec) drivesInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "storcli-drives-info")
}

// directStorCliExec executes storcli directly (Windows)
type directStorCliExec struct {
	*logger.Logger

	storcliPath string
	timeout     time.Duration
}

func newDirectStorCliExec(storcliPath string, timeout time.Duration, log *logger.Logger) *directStorCliExec {
	return &directStorCliExec{
		Logger:      log,
		storcliPath: storcliPath,
		timeout:     timeout,
	}
}

func (e *directStorCliExec) controllersInfo() ([]byte, error) {
	return ndexec.RunDirect(e.Logger, e.timeout, e.storcliPath, "/cALL", "show", "all", "J", "nolog")
}

func (e *directStorCliExec) drivesInfo() ([]byte, error) {
	return ndexec.RunDirect(e.Logger, e.timeout, e.storcliPath, "/cALL/eALL/sALL", "show", "all", "J", "nolog")
}
