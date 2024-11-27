// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricOSPhysicalMemoryFreeBytes = "windows_os_physical_memory_free_bytes"
	metricOSPagingFreeBytes         = "windows_os_paging_free_bytes"
	metricOSProcessesLimit          = "windows_os_processes_limit"
	metricOSProcesses               = "windows_os_processes"
	metricOSUsers                   = "windows_os_users"
	metricOSPagingLimitBytes        = "windows_os_paging_limit_bytes"
	metricOSVisibleMemoryBytes      = "windows_os_visible_memory_bytes"
)

func (c *Collector) collectOS(mx map[string]int64, pms prometheus.Series) {
	if !c.cache.collection[collectorOS] {
		c.cache.collection[collectorOS] = true
		c.addOSCharts()
	}

	px := "os_"
	if pm := pms.FindByName(metricOSPhysicalMemoryFreeBytes); pm.Len() > 0 {
		mx[px+"physical_memory_free_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricOSPagingFreeBytes); pm.Len() > 0 {
		mx[px+"paging_free_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricOSProcessesLimit); pm.Len() > 0 {
		mx[px+"processes_limit"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricOSProcesses); pm.Len() > 0 {
		mx[px+"processes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricOSUsers); pm.Len() > 0 {
		mx[px+"users"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricOSPagingLimitBytes); pm.Len() > 0 {
		mx[px+"paging_limit_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricOSVisibleMemoryBytes); pm.Len() > 0 {
		mx[px+"visible_memory_bytes"] = int64(pm.Max())
	}
}
