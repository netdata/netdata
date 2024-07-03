// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import (
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

func (de *DockerEngine) validateConfig() error {
	if de.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (de *DockerEngine) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(de.Client)
	if err != nil {
		return nil, err
	}
	return prometheus.New(client, de.Request), nil
}
