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
	if a.collector.client == nil {
		return nil, errDockerClientNotReady
	}
	return a.collector.client, nil
}
