// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricLogonType = "windows_logon_logon_type"
)

func (w *Windows) collectLogon(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorLogon] {
		w.cache.collection[collectorLogon] = true
		w.addLogonCharts()
	}

	for _, pm := range pms.FindByName(metricLogonType) {
		if v := pm.Labels.Get("status"); v != "" {
			mx["logon_type_"+v+"_sessions"] = int64(pm.Value)
		}
	}
}
