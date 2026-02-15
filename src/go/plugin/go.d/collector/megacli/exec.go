// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type megaCli interface {
	physDrivesInfo() ([]byte, error)
	bbuInfo() ([]byte, error)
}

// ndsudoMegaCliExec executes megacli via ndsudo (Linux/BSD)
type ndsudoMegaCliExec struct {
	*logger.Logger

	timeout time.Duration
}

func newNdsudoMegaCliExec(timeout time.Duration, log *logger.Logger) *ndsudoMegaCliExec {
	return &ndsudoMegaCliExec{
		Logger:  log,
		timeout: timeout,
	}
}

func (e *ndsudoMegaCliExec) physDrivesInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "megacli-disk-info")
}

func (e *ndsudoMegaCliExec) bbuInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "megacli-battery-info")
}

// directMegaCliExec executes megacli directly (Windows)
type directMegaCliExec struct {
	*logger.Logger

	megacliPath string
	timeout     time.Duration
}

func newDirectMegaCliExec(megacliPath string, timeout time.Duration, log *logger.Logger) *directMegaCliExec {
	return &directMegaCliExec{
		Logger:      log,
		megacliPath: megacliPath,
		timeout:     timeout,
	}
}

func (e *directMegaCliExec) physDrivesInfo() ([]byte, error) {
	return ndexec.RunDirect(e.Logger, e.timeout, e.megacliPath, "-LDPDInfo", "-aAll", "-NoLog")
}

func (e *directMegaCliExec) bbuInfo() ([]byte, error) {
	return ndexec.RunDirect(e.Logger, e.timeout, e.megacliPath, "-AdpBbuCmd", "-aAll", "-NoLog")
}
