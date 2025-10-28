// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	precision = 1000
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.cfg == nil {
		if err := c.collectConfiguration(); err != nil {
			return nil, err
		}

		c.addGlobalChartsOnce.Do(c.addGlobalCharts)
	}

	mx := make(map[string]int64)

	if err := c.collectChecks(mx); err != nil {
		return nil, err
	}

	if c.isServer() {
		if !c.isCloudManaged() {
			c.addServerAutopilotChartsOnce.Do(c.addServerAutopilotHealthCharts)
			// 'operator/autopilot/health' is disabled in Cloud managed (403: Operation is not allowed in managed Consul clusters)
			if err := c.collectAutopilotHealth(mx); err != nil {
				return nil, err
			}
		}
		if err := c.collectNetworkRTT(mx); err != nil {
			return nil, err
		}
	}

	if c.isTelemetryPrometheusEnabled() {
		if err := c.collectMetricsPrometheus(mx); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (c *Collector) isTelemetryPrometheusEnabled() bool {
	return c.cfg.DebugConfig.Telemetry.PrometheusOpts.Expiration != "0s"
}

func (c *Collector) isCloudManaged() bool {
	return c.cfg.DebugConfig.Cloud.ClientSecret != "" || c.cfg.DebugConfig.Cloud.ResourceID != ""
}

func (c *Collector) hasLicense() bool {
	return c.cfg.Stats.License.ID != ""
}

func (c *Collector) isServer() bool {
	return c.cfg.Config.Server
}

func (c *Collector) client(statusCodes ...int) *web.Client {
	return web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		return slices.Contains(statusCodes, resp.StatusCode), nil
	})
}

func (c *Collector) createRequest(urlPath string) (*http.Request, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create '%s' request: %w", urlPath, err)
	}

	if c.ACLToken != "" {
		req.Header.Set("X-Consul-Token", c.ACLToken)
	}

	return req, nil
}
