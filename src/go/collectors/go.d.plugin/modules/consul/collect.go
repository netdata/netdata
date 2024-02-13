// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/go.d.plugin/pkg/web"
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
	req, err := web.NewHTTPRequest(c.Request.Copy())
	if err != nil {
		return fmt.Errorf("error on creating request: %v", err)
	}

	req.URL.Path = urlPath
	if c.ACLToken != "" {
		req.Header.Set("X-Consul-Token", c.ACLToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on request to %s : %v", req.URL, err)
	}

	defer closeBody(resp)

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

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
