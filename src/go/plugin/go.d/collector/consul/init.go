// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (c *Collector) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.ClientConfig)
}

const urlPathAgentMetrics = "/v1/agent/metrics"

func (c *Collector) initPrometheusClient(httpClient *http.Client) (prometheus.Prometheus, error) {
	r, err := web.NewHTTPRequest(c.RequestConfig.Copy())
	if err != nil {
		return nil, err
	}
	r.URL.Path = urlPathAgentMetrics
	r.URL.RawQuery = url.Values{
		"format": []string{"prometheus"},
	}.Encode()

	req := c.RequestConfig.Copy()
	req.URL = r.URL.String()

	if c.ACLToken != "" {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers["X-Consul-Token"] = c.ACLToken
	}

	return prometheus.New(httpClient, req), nil
}
