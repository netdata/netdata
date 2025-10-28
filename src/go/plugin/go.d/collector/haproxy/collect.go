// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	metricBackendSessionsTotal              = "haproxy_backend_sessions_total"
	metricBackendCurrentSessions            = "haproxy_backend_current_sessions"
	metricBackendHTTPResponsesTotal         = "haproxy_backend_http_responses_total"
	metricBackendResponseTimeAverageSeconds = "haproxy_backend_response_time_average_seconds"
	metricBackendCurrentQueue               = "haproxy_backend_current_queue"
	metricBackendQueueTimeAverageSeconds    = "haproxy_backend_queue_time_average_seconds"
	metricBackendBytesInTotal               = "haproxy_backend_bytes_in_total"
	metricBackendBytesOutTotal              = "haproxy_backend_bytes_out_total"
)

func isHaproxyMetrics(pms prometheus.Series) bool {
	for _, pm := range pms {
		if strings.HasPrefix(pm.Name(), "haproxy_") {
			return true
		}
	}
	return false
}

func (c *Collector) collect() (map[string]int64, error) {
	pms, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if c.validateMetrics && !isHaproxyMetrics(pms) {
		return nil, errors.New("unexpected metrics (not HAProxy)")
	}
	c.validateMetrics = false

	mx := make(map[string]int64)
	for _, pm := range pms {
		proxy := pm.Labels.Get("proxy")
		if proxy == "" {
			continue
		}

		if !c.proxies[proxy] {
			c.proxies[proxy] = true
			c.addProxyToCharts(proxy)
		}

		mx[dimID(pm)] = int64(pm.Value * multiplier(pm))
	}

	return mx, nil
}

func (c *Collector) addProxyToCharts(proxy string) {
	c.addDimToChart(chartBackendCurrentSessions.ID, &module.Dim{
		ID:   proxyDimID(metricBackendCurrentSessions, proxy),
		Name: proxy,
	})
	c.addDimToChart(chartBackendSessions.ID, &module.Dim{
		ID:   proxyDimID(metricBackendSessionsTotal, proxy),
		Name: proxy,
		Algo: module.Incremental,
	})

	c.addDimToChart(chartBackendResponseTimeAverage.ID, &module.Dim{
		ID:   proxyDimID(metricBackendResponseTimeAverageSeconds, proxy),
		Name: proxy,
	})
	if err := c.Charts().Add(newChartBackendHTTPResponses(proxy)); err != nil {
		c.Warning(err)
	}

	c.addDimToChart(chartBackendCurrentQueue.ID, &module.Dim{
		ID:   proxyDimID(metricBackendCurrentQueue, proxy),
		Name: proxy,
	})
	c.addDimToChart(chartBackendQueueTimeAverage.ID, &module.Dim{
		ID:   proxyDimID(metricBackendQueueTimeAverageSeconds, proxy),
		Name: proxy,
	})

	if err := c.Charts().Add(newChartBackendNetworkIO(proxy)); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addDimToChart(chartID string, dim *module.Dim) {
	chart := c.Charts().Get(chartID)
	if chart == nil {
		c.Warningf("error on adding '%s' dimension: can not find '%s' chart", dim.ID, chartID)
		return
	}
	if err := chart.AddDim(dim); err != nil {
		c.Warning(err)
		return
	}
	chart.MarkNotCreated()
}

func multiplier(pm prometheus.SeriesSample) float64 {
	switch pm.Name() {
	case metricBackendResponseTimeAverageSeconds,
		metricBackendQueueTimeAverageSeconds:
		// to milliseconds
		return 1000
	}
	return 1
}

func dimID(pm prometheus.SeriesSample) string {
	proxy := pm.Labels.Get("proxy")
	if proxy == "" {
		return ""
	}

	name := cleanMetricName(pm.Name())
	if pm.Name() == metricBackendHTTPResponsesTotal {
		name += "_" + pm.Labels.Get("code")
	}
	return proxyDimID(name, proxy)
}

func proxyDimID(metric, proxy string) string {
	return cleanMetricName(metric) + "_proxy_" + proxy
}

func cleanMetricName(name string) string {
	if strings.HasSuffix(name, "_total") {
		return name[:len(name)-6]
	}
	if strings.HasSuffix(name, "_seconds") {
		return name[:len(name)-8]
	}
	return name
}
