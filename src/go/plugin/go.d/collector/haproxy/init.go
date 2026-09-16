// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig(ctx context.Context) error {
	if c.URL == "" {
		return errors.New("'url' is not set")
	}
	if _, err := web.NewHTTPRequest(ctx, c.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (c *Collector) initPrometheusClient(ctx context.Context) (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(ctx, c.ClientConfig)
	if err != nil {
		return nil, err
	}

	prom := prometheus.NewWithSelector(httpClient, c.RequestConfig, sr)
	return prom, nil
}

var sr, _ = selector.Expr{
	Allow: []string{
		metricBackendHTTPResponsesTotal,
		metricBackendCurrentQueue,
		metricBackendQueueTimeAverageSeconds,
		metricBackendBytesInTotal,
		metricBackendResponseTimeAverageSeconds,
		metricBackendSessionsTotal,
		metricBackendCurrentSessions,
		metricBackendBytesOutTotal,
	},
}.Parse()
