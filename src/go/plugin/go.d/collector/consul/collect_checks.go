// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

const (
	// https://www.consul.io/api-docs/agent/check#list-checks
	urlPathAgentChecks = "/v1/agent/checks"
)

type agentCheck struct {
	Node        string
	CheckID     string
	Name        string
	Status      string
	ServiceID   string
	ServiceName string
	ServiceTags []string
}

func (c *Collector) collectChecks(mx map[string]int64) error {
	req, err := c.createRequest(urlPathAgentChecks)
	if err != nil {
		return err
	}

	var checks map[string]*agentCheck

	if err := c.client().RequestJSON(req, &checks); err != nil {
		return err
	}

	for id, check := range checks {
		if !c.checks[id] {
			c.checks[id] = true
			c.addHealthCheckCharts(check)
		}

		mx["health_check_"+id+"_passing_status"] = metrix.Bool(check.Status == "passing")
		mx["health_check_"+id+"_warning_status"] = metrix.Bool(check.Status == "warning")
		mx["health_check_"+id+"_critical_status"] = metrix.Bool(check.Status == "critical")
		mx["health_check_"+id+"_maintenance_status"] = metrix.Bool(check.Status == "maintenance")
	}

	for id := range c.checks {
		if _, ok := checks[id]; !ok {
			delete(c.checks, id)
			c.removeHealthCheckCharts(id)
		}
	}

	return nil
}
