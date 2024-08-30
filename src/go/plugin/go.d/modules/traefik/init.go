// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (t *Traefik) validateConfig() error {
	if t.URL == "" {
		return errors.New("'url' is not set")
	}
	return nil
}

func (t *Traefik) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(t.Client)
	if err != nil {
		return nil, err
	}

	prom := prometheus.NewWithSelector(httpClient, t.Request, sr)
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
