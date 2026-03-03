// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/docker/dockerfunc"
)

var errDockerClientNotReady = errors.New("collector docker client is not ready")

type funcDepsAdapter struct {
	collector *Collector
}

func (a funcDepsAdapter) DockerClient() (dockerfunc.DockerClient, error) {
	return a.collector.currentDockerClient()
}

func (c *Collector) dockerClientReady() bool {
	return c.client != nil
}

func (c *Collector) currentDockerClient() (dockerfunc.DockerClient, error) {
	if c.client == nil {
		return nil, errDockerClientNotReady
	}
	return c.client, nil
}

var _ dockerfunc.Deps = (*funcDepsAdapter)(nil)
