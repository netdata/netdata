// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Consul) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (c *Consul) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.Client)
}

const urlPathAgentMetrics = "/v1/agent/metrics"

func (c *Consul) initPrometheusClient(httpClient *http.Client) (prometheus.Prometheus, error) {
	r, err := web.NewHTTPRequest(c.Request.Copy())
	if err != nil {
		return nil, err
	}
	r.URL.Path = urlPathAgentMetrics
	r.URL.RawQuery = url.Values{
		"format": []string{"prometheus"},
	}.Encode()

	req := c.Request.Copy()
	req.URL = r.URL.String()

	if c.ACLToken != "" {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers["X-Consul-Token"] = c.ACLToken
	}

	return prometheus.New(httpClient, req), nil
}
