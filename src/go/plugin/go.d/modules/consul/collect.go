// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	precision = 1000
)

func (c *Consul) collect() (map[string]int64, error) {
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

func (c *Consul) isTelemetryPrometheusEnabled() bool {
	return c.cfg.DebugConfig.Telemetry.PrometheusOpts.Expiration != "0s"
}

func (c *Consul) isCloudManaged() bool {
	return c.cfg.DebugConfig.Cloud.ClientSecret != "" || c.cfg.DebugConfig.Cloud.ResourceID != ""
}

func (c *Consul) hasLicense() bool {
	return c.cfg.Stats.License.ID != ""
}

func (c *Consul) isServer() bool {
	return c.cfg.Config.Server
}

func (c *Consul) doOKDecode(urlPath string, in interface{}, statusCodes ...int) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPath)
	if err != nil {
		return fmt.Errorf("error on creating request: %v", err)
	}

	if c.ACLToken != "" {
		req.Header.Set("X-Consul-Token", c.ACLToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on request to %s : %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	codes := map[int]bool{http.StatusOK: true}
	for _, v := range statusCodes {
		codes[v] = true
	}

	if !codes[resp.StatusCode] {
		return fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}

	if err = json.NewDecoder(resp.Body).Decode(&in); err != nil {
		return fmt.Errorf("error on decoding response from %s : %v", req.URL, err)
	}

	return nil
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
