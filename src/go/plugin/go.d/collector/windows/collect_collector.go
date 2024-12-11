// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricCollectorDuration = "windows_exporter_collector_duration_seconds"
	metricCollectorSuccess  = "windows_exporter_collector_success"
)

func (c *Collector) collectCollector(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "collector_"
	for _, pm := range pms.FindByName(metricCollectorDuration) {
		if name := pm.Labels.Get("collector"); name != "" {
			seen[name] = true
			mx[px+name+"_duration"] = int64(pm.Value * precision)
		}
	}
	for _, pm := range pms.FindByName(metricCollectorSuccess) {
		if name := pm.Labels.Get("collector"); name != "" {
			seen[name] = true
			if pm.Value == 1 {
				mx[px+name+"_status_success"], mx[px+name+"_status_fail"] = 1, 0
			} else {
				mx[px+name+"_status_success"], mx[px+name+"_status_fail"] = 0, 1
			}
		}
	}

	for name := range seen {
		if !c.cache.collectors[name] {
			c.cache.collectors[name] = true
			c.addCollectorCharts(name)
		}
	}
	for name := range c.cache.collectors {
		if !seen[name] {
			delete(c.cache.collectors, name)
			c.removeCollectorCharts(name)
		}
	}
}
