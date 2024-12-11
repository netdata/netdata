// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricSysSystemUpTime = "windows_system_system_up_time"
	metricSysThreads      = "windows_system_threads"
)

func (c *Collector) collectSystem(mx map[string]int64, pms prometheus.Series) {
	if !c.cache.collection[collectorSystem] {
		c.cache.collection[collectorSystem] = true
		c.addSystemCharts()
	}

	px := "system_"
	if pm := pms.FindByName(metricSysSystemUpTime); pm.Len() > 0 {
		mx[px+"up_time"] = time.Now().Unix() - int64(pm.Max())
	}
	if pm := pms.FindByName(metricSysThreads); pm.Len() > 0 {
		mx[px+"threads"] = int64(pm.Max())
	}
}
