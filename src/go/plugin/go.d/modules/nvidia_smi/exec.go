// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newNvidiaSMIExec(path string, cfg Config, log *logger.Logger) (*nvidiaSMIExec, error) {
	return &nvidiaSMIExec{
		binPath: path,
		timeout: cfg.Timeout.Duration(),
		Logger:  log,
	}, nil
}

type nvidiaSMIExec struct {
	binPath string
	timeout time.Duration
	*logger.Logger
}

func (e *nvidiaSMIExec) queryGPUInfoXML() ([]byte, error) {
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

func (e *nvidiaSMIExec) queryGPUInfoCSV(properties []string) ([]byte, error) {
	if len(properties) == 0 {
		return nil, errors.New("can not query CSV GPU Info without properties")
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "--query-gpu="+strings.Join(properties, ","), "--format=csv,nounits")

	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func (e *nvidiaSMIExec) queryHelpQueryGPU() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "--help-query-gpu")

	e.Debugf("executing '%s'", cmd)
	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, err
}
