// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"
)

func (c *Collector) syncTopologyChartReferences() {
	if c == nil || c.topologyCache == nil {
		return
	}

	deviceCharts := c.collectLocalDeviceCharts()
	interfaceCharts := c.collectLocalInterfaceCharts()

	c.topologyCache.mu.Lock()
	defer c.topologyCache.mu.Unlock()

	local := c.topologyCache.localDevice
	if local.ChartIDPrefix == "" {
		local.ChartIDPrefix = topologyProfileChartIDPrefix
	}
	if local.ChartContextPrefix == "" {
		local.ChartContextPrefix = topologyProfileChartContextPrefix
	}
	if c.vnode != nil && strings.TrimSpace(c.vnode.GUID) != "" {
		local.NetdataHostID = strings.TrimSpace(c.vnode.GUID)
	}
	local.DeviceCharts = deviceCharts
	local.InterfaceCharts = interfaceCharts
	c.topologyCache.localDevice = local
}

func (c *Collector) collectLocalDeviceCharts() map[string]string {
	if c == nil || c.charts == nil {
		return nil
	}

	out := make(map[string]string)
	staticCharts := map[string]string{
		"ping_rtt":         "ping_rtt",
		"ping_rtt_stddev":  "ping_rtt_stddev",
		"topology_devices": "topology_devices",
		"topology_links":   "topology_links",
	}
	for semantic, chartID := range staticCharts {
		if c.chartExists(chartID) {
			out[semantic] = chartID
		}
	}
	for metricName := range c.seenScalarMetrics {
		metricName = strings.TrimSpace(metricName)
		if metricName == "" {
			continue
		}
		chartID := topologyProfileChartIDPrefix + cleanMetricName.Replace(metricName)
		if c.chartExists(chartID) {
			out[metricName] = chartID
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Collector) collectLocalInterfaceCharts() map[string]topologyInterfaceChartRef {
	if c == nil || c.ifaceCache == nil {
		return nil
	}

	c.ifaceCache.mu.RLock()
	defer c.ifaceCache.mu.RUnlock()

	out := make(map[string]topologyInterfaceChartRef)
	for _, entry := range c.ifaceCache.interfaces {
		if entry == nil {
			continue
		}
		suffix := strings.TrimSpace(entry.name)
		if suffix == "" {
			continue
		}

		availableMetrics := make([]string, 0, len(entry.availableMetrics))
		for metricName := range entry.availableMetrics {
			metricName = strings.TrimSpace(metricName)
			if metricName == "" {
				continue
			}
			chartID := topologyProfileChartIDPrefix + cleanMetricName.Replace(metricName+"_"+suffix)
			if c.chartExists(chartID) {
				availableMetrics = append(availableMetrics, metricName)
			}
		}
		sort.Strings(availableMetrics)
		if len(availableMetrics) == 0 {
			continue
		}

		out[suffix] = topologyInterfaceChartRef{
			ChartIDSuffix:    suffix,
			AvailableMetrics: deduplicateSortedStrings(availableMetrics),
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Collector) chartExists(chartID string) bool {
	if c == nil || c.charts == nil {
		return false
	}
	chart := c.charts.Get(strings.TrimSpace(chartID))
	return chart != nil && !chart.Obsolete
}
