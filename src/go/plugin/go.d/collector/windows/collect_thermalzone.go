// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricThermalzoneTemperatureCelsius = "windows_thermalzone_temperature_celsius"
)

func (c *Collector) collectThermalzone(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	for _, pm := range pms.FindByName(metricThermalzoneTemperatureCelsius) {
		if name := cleanZoneName(pm.Labels.Get("name")); name != "" {
			seen[name] = true
			mx["thermalzone_"+name+"_temperature"] = int64(pm.Value)
		}
	}

	for zone := range seen {
		if !c.cache.thermalZones[zone] {
			c.cache.thermalZones[zone] = true
			c.addThermalZoneCharts(zone)
		}
	}
	for zone := range c.cache.thermalZones {
		if !seen[zone] {
			delete(c.cache.thermalZones, zone)
			c.removeThermalZoneCharts(zone)
		}
	}
}

func cleanZoneName(name string) string {
	// "\\_TZ.TZ10", "\\_TZ.X570" => TZ10, X570
	i := strings.Index(name, ".")
	if i == -1 || len(name) == i+1 {
		return ""
	}
	return name[i+1:]
}
