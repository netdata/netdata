// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
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

func (h *Haproxy) collect() (map[string]int64, error) {
	pms, err := h.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if h.validateMetrics && !isHaproxyMetrics(pms) {
		return nil, errors.New("unexpected metrics (not HAProxy)")
	}
	h.validateMetrics = false

	mx := make(map[string]int64)
	for _, pm := range pms {
		proxy := pm.Labels.Get("proxy")
		if proxy == "" {
			continue
		}

		if !h.proxies[proxy] {
			h.proxies[proxy] = true
			h.addProxyToCharts(proxy)
		}

		mx[dimID(pm)] = int64(pm.Value * multiplier(pm))
	}

	return mx, nil
}

func (h *Haproxy) addProxyToCharts(proxy string) {
	h.addDimToChart(chartBackendCurrentSessions.ID, &module.Dim{
		ID:   proxyDimID(metricBackendCurrentSessions, proxy),
		Name: proxy,
	})
	h.addDimToChart(chartBackendSessions.ID, &module.Dim{
		ID:   proxyDimID(metricBackendSessionsTotal, proxy),
		Name: proxy,
		Algo: module.Incremental,
	})

	h.addDimToChart(chartBackendResponseTimeAverage.ID, &module.Dim{
		ID:   proxyDimID(metricBackendResponseTimeAverageSeconds, proxy),
		Name: proxy,
	})
	if err := h.Charts().Add(newChartBackendHTTPResponses(proxy)); err != nil {
		h.Warning(err)
	}

	h.addDimToChart(chartBackendCurrentQueue.ID, &module.Dim{
		ID:   proxyDimID(metricBackendCurrentQueue, proxy),
		Name: proxy,
	})
	h.addDimToChart(chartBackendQueueTimeAverage.ID, &module.Dim{
		ID:   proxyDimID(metricBackendQueueTimeAverageSeconds, proxy),
		Name: proxy,
	})

	if err := h.Charts().Add(newChartBackendNetworkIO(proxy)); err != nil {
		h.Warning(err)
	}
}

func (h *Haproxy) addDimToChart(chartID string, dim *module.Dim) {
	chart := h.Charts().Get(chartID)
	if chart == nil {
		h.Warningf("error on adding '%s' dimension: can not find '%s' chart", dim.ID, chartID)
		return
	}
	if err := chart.AddDim(dim); err != nil {
		h.Warning(err)
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
