// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (c *Cassandra) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' is not set")
	}
	return nil
}

func (c *Cassandra) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(c.Client)
	if err != nil {
		return nil, err
	}
	return prometheus.New(client, c.Request), nil
}
