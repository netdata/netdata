// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type arcconfCli interface {
	logicalDevicesInfo() ([]byte, error)
	physicalDevicesInfo() ([]byte, error)
}

// ndsudoArcconfCliExec executes arcconf via ndsudo (Linux/BSD)
type ndsudoArcconfCliExec struct {
	*logger.Logger
	timeout time.Duration
}

func newNdsudoArcconfCliExec(timeout time.Duration, log *logger.Logger) *ndsudoArcconfCliExec {
	return &ndsudoArcconfCliExec{
		Logger:  log,
		timeout: timeout,
	}
}

func (e *ndsudoArcconfCliExec) logicalDevicesInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "arcconf-ld-info")
}

func (e *ndsudoArcconfCliExec) physicalDevicesInfo() ([]byte, error) {
	return ndexec.RunNDSudo(e.Logger, e.timeout, "arcconf-pd-info")
}

// directArcconfCliExec executes arcconf directly (Windows)
type directArcconfCliExec struct {
	*logger.Logger

	arcconfPath string
	timeout     time.Duration
}

func newDirectArcconfCliExec(arcconfPath string, timeout time.Duration, log *logger.Logger) *directArcconfCliExec {
	return &directArcconfCliExec{
		Logger:      log,
		arcconfPath: arcconfPath,
		timeout:     timeout,
	}
}

func (e *directArcconfCliExec) logicalDevicesInfo() ([]byte, error) {
	return e.execute("GETCONFIG", "1", "LD")
}

func (e *directArcconfCliExec) physicalDevicesInfo() ([]byte, error) {
	return e.execute("GETCONFIG", "1", "PD")
}

func (e *directArcconfCliExec) execute(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.arcconfPath, args...)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("'%s' execution failed: %v", cmd, err)
	}

	return bs, nil
}
