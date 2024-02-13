// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/go.d.plugin/pkg/prometheus"
)

const (
	metricServiceState  = "windows_service_state"
	metricServiceStatus = "windows_service_status"
)

func (w *Windows) collectService(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "service_"
	for _, pm := range pms.FindByName(metricServiceState) {
		name := cleanService(pm.Labels.Get("name"))
		state := cleanService(pm.Labels.Get("state"))
		if name == "" || state == "" {
			continue
		}

		seen[name] = true
		mx[px+name+"_state_"+state] = int64(pm.Value)
	}
	for _, pm := range pms.FindByName(metricServiceStatus) {
		name := cleanService(pm.Labels.Get("name"))
		status := cleanService(pm.Labels.Get("status"))
		if name == "" || status == "" {
			continue
		}

		seen[name] = true
		mx[px+name+"_status_"+status] = int64(pm.Value)
	}

	for svc := range seen {
		if !w.cache.services[svc] {
			w.cache.services[svc] = true
			w.addServiceCharts(svc)
		}
	}
	for svc := range w.cache.services {
		if !seen[svc] {
			delete(w.cache.services, svc)
			w.removeServiceCharts(svc)
		}
	}
}

func cleanService(name string) string {
	return strings.ReplaceAll(name, " ", "_")
}
