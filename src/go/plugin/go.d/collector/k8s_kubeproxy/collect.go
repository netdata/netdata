// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

import (
	"math"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collect() (map[string]int64, error) {
	raw, err := c.prom.ScrapeSeries()

	if err != nil {
		return nil, err
	}

	mx := newMetrics()

	c.collectSyncProxyRules(raw, mx)
	c.collectRESTClientHTTPRequests(raw, mx)
	c.collectHTTPRequestDuration(raw, mx)

	return stm.ToMap(mx), nil
}

func (c *Collector) collectSyncProxyRules(raw prometheus.Series, mx *metrics) {
	m := raw.FindByName("kubeproxy_sync_proxy_rules_latency_microseconds_count")
	mx.SyncProxyRules.Count.Set(m.Max())
	c.collectSyncProxyRulesLatency(raw, mx)
}

func (c *Collector) collectSyncProxyRulesLatency(raw prometheus.Series, mx *metrics) {
	metricName := "kubeproxy_sync_proxy_rules_latency_microseconds_bucket"
	latency := &mx.SyncProxyRules.Latency

	for _, metric := range raw.FindByName(metricName) {
		value := metric.Value
		bucket := strings.TrimSuffix(metric.Labels.Get("le"), ".0")
		switch bucket {
		case "1000":
			latency.LE1000.Set(value)
		case "2000":
			latency.LE2000.Set(value)
		case "4000":
			latency.LE4000.Set(value)
		case "8000":
			latency.LE8000.Set(value)
		case "16000":
			latency.LE16000.Set(value)
		case "32000":
			latency.LE32000.Set(value)
		case "64000":
			latency.LE64000.Set(value)
		case "128000":
			latency.LE128000.Set(value)
		case "256000":
			latency.LE256000.Set(value)
		case "512000":
			latency.LE512000.Set(value)
		case "1.024e+06":
			latency.LE1024000.Set(value)
		case "2.048e+06":
			latency.LE2048000.Set(value)
		case "4.096e+06":
			latency.LE4096000.Set(value)
		case "8.192e+06":
			latency.LE8192000.Set(value)
		case "1.6384e+07":
			latency.LE16384000.Set(value)
		case "+Inf":
			latency.Inf.Set(value)
		}
	}

	latency.Inf.Sub(latency.LE16384000.Value())
	latency.LE16384000.Sub(latency.LE8192000.Value())
	latency.LE8192000.Sub(latency.LE4096000.Value())
	latency.LE4096000.Sub(latency.LE2048000.Value())
	latency.LE2048000.Sub(latency.LE1024000.Value())
	latency.LE1024000.Sub(latency.LE512000.Value())
	latency.LE512000.Sub(latency.LE256000.Value())
	latency.LE256000.Sub(latency.LE128000.Value())
	latency.LE128000.Sub(latency.LE64000.Value())
	latency.LE64000.Sub(latency.LE32000.Value())
	latency.LE32000.Sub(latency.LE16000.Value())
	latency.LE16000.Sub(latency.LE8000.Value())
	latency.LE8000.Sub(latency.LE4000.Value())
	latency.LE4000.Sub(latency.LE2000.Value())
	latency.LE2000.Sub(latency.LE1000.Value())
}

func (c *Collector) collectRESTClientHTTPRequests(raw prometheus.Series, mx *metrics) {
	metricName := "rest_client_requests_total"
	chart := c.charts.Get("rest_client_requests_by_code")

	for _, metric := range raw.FindByName(metricName) {
		code := metric.Labels.Get("code")
		if code == "" {
			continue
		}
		dimID := "rest_client_requests_" + code
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: code, Algo: module.Incremental})
			chart.MarkNotCreated()
		}
		mx.RESTClient.Requests.ByStatusCode[code] = mtx.Gauge(metric.Value)
	}

	chart = c.charts.Get("rest_client_requests_by_method")

	for _, metric := range raw.FindByName(metricName) {
		method := metric.Labels.Get("method")
		if method == "" {
			continue
		}
		dimID := "rest_client_requests_" + method
		if !chart.HasDim(dimID) {
			_ = chart.AddDim(&Dim{ID: dimID, Name: method, Algo: module.Incremental})
			chart.MarkNotCreated()
		}
		mx.RESTClient.Requests.ByMethod[method] = mtx.Gauge(metric.Value)
	}
}

func (c *Collector) collectHTTPRequestDuration(raw prometheus.Series, mx *metrics) {
	// Summary
	for _, metric := range raw.FindByName("http_request_duration_microseconds") {
		if math.IsNaN(metric.Value) {
			continue
		}
		quantile := metric.Labels.Get("quantile")
		switch quantile {
		case "0.5":
			mx.HTTP.Request.Duration.Quantile05.Set(metric.Value)
		case "0.9":
			mx.HTTP.Request.Duration.Quantile09.Set(metric.Value)
		case "0.99":
			mx.HTTP.Request.Duration.Quantile099.Set(metric.Value)
		}
	}
}
