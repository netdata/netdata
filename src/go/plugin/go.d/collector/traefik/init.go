// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' is not set")
	}
	return nil
}

func (c *Collector) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}

	prom := prometheus.NewWithSelector(httpClient, c.RequestConfig, sr)
	return prom, nil
}

var sr, _ = selector.Expr{
	Allow: []string{
		metricEntrypointRequestDurationSecondsSum,
		metricEntrypointRequestDurationSecondsCount,
		metricEntrypointRequestsTotal,
		metricEntrypointOpenConnections,
	},
}.Parse()
