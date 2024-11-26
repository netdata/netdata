// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (h *Haproxy) validateConfig() error {
	if h.URL == "" {
		return errors.New("'url' is not set")
	}
	if _, err := web.NewHTTPRequest(h.RequestConfig); err != nil {
		return err
	}
	return nil
}

func (h *Haproxy) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(h.ClientConfig)
	if err != nil {
		return nil, err
	}

	prom := prometheus.NewWithSelector(httpClient, h.RequestConfig, sr)
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
