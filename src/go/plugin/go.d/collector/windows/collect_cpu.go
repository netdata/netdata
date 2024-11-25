// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricCPUTimeTotal       = "windows_cpu_time_total"
	metricCPUInterruptsTotal = "windows_cpu_interrupts_total"
	metricCPUDPCsTotal       = "windows_cpu_dpcs_total"
	metricCPUCStateTotal     = "windows_cpu_cstate_seconds_total"
)

func (w *Windows) collectCPU(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorCPU] {
		w.cache.collection[collectorCPU] = true
		w.addCPUCharts()
	}

	seen := make(map[string]bool)
	for _, pm := range pms.FindByName(metricCPUTimeTotal) {
		core := pm.Labels.Get("core")
		mode := pm.Labels.Get("mode")
		if core == "" || mode == "" {
			continue
		}

		seen[core] = true
		mx["cpu_"+mode+"_time"] += int64(pm.Value * precision)
		mx["cpu_core_"+core+"_"+mode+"_time"] += int64(pm.Value * precision)
	}

	for _, pm := range pms.FindByName(metricCPUInterruptsTotal) {
		core := pm.Labels.Get("core")
		if core == "" {
			continue
		}

		seen[core] = true
		mx["cpu_core_"+core+"_interrupts"] += int64(pm.Value)
	}

	for _, pm := range pms.FindByName(metricCPUDPCsTotal) {
		core := pm.Labels.Get("core")
		if core == "" {
			continue
		}

		seen[core] = true
		mx["cpu_core_"+core+"_dpcs"] += int64(pm.Value)
	}

	for _, pm := range pms.FindByName(metricCPUCStateTotal) {
		core := pm.Labels.Get("core")
		state := pm.Labels.Get("state")
		if core == "" || state == "" {
			continue
		}

		seen[core] = true
		mx["cpu_core_"+core+"_cstate_"+state] += int64(pm.Value * precision)
	}

	for core := range seen {
		if !w.cache.cores[core] {
			w.cache.cores[core] = true
			w.addCPUCoreCharts(core)
		}
	}
	for core := range w.cache.cores {
		if !seen[core] {
			delete(w.cache.cores, core)
			w.removeCPUCoreCharts(core)
		}
	}
}
