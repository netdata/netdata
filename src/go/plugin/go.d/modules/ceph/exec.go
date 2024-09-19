// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type cephBinary interface {
	df() ([]byte, error)
	osdPoolStats() ([]byte, error)
	osdDf() ([]byte, error)
	osdPerf() ([]byte, error)
}

func newCephExecBinary(binPath string, cfg Config, log *logger.Logger) cephBinary {
	return &cephExec{
		Logger:  log,
		binPath: binPath,
		timeout: cfg.Timeout.Duration(),
	}
}

type cephExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *cephExec) df() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "ceph-df", "df", "--format", "json")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func (e *cephExec) osdDf() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "ceph-osd-df", "osd", "df", "--format", "json")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func (e *cephExec) osdPerf() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "ceph-osd-perf", "osd", "perf", "--format", "json")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func (e *cephExec) osdPoolStats() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, "ceph-osd-pool-stats", "osd", "pool", "stats", "--format", "json")
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
