// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Cassandra) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' is not set")
	}
	return nil
}

func (c *Cassandra) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}
	return prometheus.New(client, c.RequestConfig), nil
}
