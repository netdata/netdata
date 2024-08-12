// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type nvidiaSmiBinary interface {
	queryGPUInfo() ([]byte, error)
}

func newNvidiaSmiExec(path string, cfg Config, log *logger.Logger) (*nvidiaSmiExec, error) {
	return &nvidiaSmiExec{
		Logger:  log,
		binPath: path,
		timeout: cfg.Timeout.Duration(),
	}, nil
}

type nvidiaSmiExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *nvidiaSmiExec) queryGPUInfo() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "-q", "-x")

	e.Debugf("executing '%s'", cmd)
	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
