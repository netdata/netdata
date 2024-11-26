// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricLogonType = "windows_logon_logon_type"
)

func (c *Collector) collectLogon(mx map[string]int64, pms prometheus.Series) {
	if !c.cache.collection[collectorLogon] {
		c.cache.collection[collectorLogon] = true
		c.addLogonCharts()
	}

	for _, pm := range pms.FindByName(metricLogonType) {
		if v := pm.Labels.Get("status"); v != "" {
			mx["logon_type_"+v+"_sessions"] = int64(pm.Value)
		}
	}
}
