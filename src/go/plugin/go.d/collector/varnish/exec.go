// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"context"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/dockerhost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type varnishstatBinary interface {
	statistics() ([]byte, error)
}

func newVarnishstatExecBinary(binPath string, cfg Config, log *logger.Logger) varnishstatBinary {
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
	return ndexec.RunUnprivileged(e.Logger, e.timeout, "varnishstat-stats", "--instanceName", e.instanceName)
}

func newVarnishstatDockerExecBinary(cfg Config, log *logger.Logger) varnishstatBinary {
	return &varnishstatDockerExec{
		Logger:       log,
		timeout:      cfg.Timeout.Duration(),
		instanceName: cfg.InstanceName,
		container:    cfg.DockerContainer,
	}
}

type varnishstatDockerExec struct {
	*logger.Logger

	timeout      time.Duration
	instanceName string
	container    string
}

func (e *varnishstatDockerExec) statistics() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	timeS := strconv.Itoa(max(int(e.timeout.Seconds()), 1))

	args := []string{"-1", "-t", timeS}
	if e.instanceName != "" {
		args = append(args, "-n", e.instanceName)
	}

	return dockerhost.Exec(ctx, e.container, "varnishstat", args...)
}
