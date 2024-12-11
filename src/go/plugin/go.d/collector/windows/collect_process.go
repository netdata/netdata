// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricProcessCPUTimeTotal    = "windows_process_cpu_time_total"
	metricProcessWorkingSetBytes = "windows_process_working_set_private_bytes"
	metricProcessIOBytes         = "windows_process_io_bytes_total"
	metricProcessIOOperations    = "windows_process_io_operations_total"
	metricProcessPageFaults      = "windows_process_page_faults_total"
	metricProcessPageFileBytes   = "windows_process_page_file_bytes"
	metricProcessThreads         = "windows_process_threads"
	metricProcessCPUHandles      = "windows_process_handles"
)

func (c *Collector) collectProcess(mx map[string]int64, pms prometheus.Series) {
	if !c.cache.collection[collectorProcess] {
		c.cache.collection[collectorProcess] = true
		c.addProcessesCharts()
	}

	seen := make(map[string]bool)
	px := "process_"
	for _, pm := range pms.FindByName(metricProcessCPUTimeTotal) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_cpu_time"] += int64(pm.Value * 1000)
		}
	}
	for _, pm := range pms.FindByName(metricProcessWorkingSetBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_working_set_private_bytes"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricProcessIOBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_io_bytes"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricProcessIOOperations) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_io_operations"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricProcessPageFaults) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_page_faults"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricProcessPageFileBytes) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_page_file_bytes"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricProcessThreads) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_threads"] += int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricProcessCPUHandles) {
		if name := cleanProcessName(pm.Labels.Get("process")); name != "" {
			seen[name] = true
			mx[px+name+"_handles"] += int64(pm.Value)
		}
	}

	for proc := range seen {
		if !c.cache.processes[proc] {
			c.cache.processes[proc] = true
			c.addProcessToCharts(proc)
		}
	}
	for proc := range c.cache.processes {
		if !seen[proc] {
			delete(c.cache.processes, proc)
			c.removeProcessFromCharts(proc)
		}
	}
}

func cleanProcessName(name string) string {
	return strings.ReplaceAll(name, " ", "_")
}
