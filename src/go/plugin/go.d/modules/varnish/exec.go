// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type varnishstatBinary interface {
	statistics() ([]byte, error)
}

func newVarnishstatBinary(binPath string, cfg Config, log *logger.Logger) varnishstatBinary {
	return &varnishstatExec{
		Logger:       log,
		binPath:      binPath,
		timeout:      cfg.Timeout.Duration(),
		instanceName: cfg.InstanceName,
	}
}

type varnishstatExec struct {
	*logger.Logger

	binPath      string
	timeout      time.Duration
	instanceName string
}

func (e *varnishstatExec) statistics() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "varnishstat-stats", "--instanceName", e.instanceName)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
