// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const precision = 1000

const (
	collectorAD                             = "ad"
	collectorADCS                           = "adcs"
	collectorADFS                           = "adfs"
	collectorCPU                            = "cpu"
	collectorMemory                         = "memory"
	collectorNet                            = "net"
	collectorLogicalDisk                    = "logical_disk"
	collectorOS                             = "os"
	collectorSystem                         = "system"
	collectorLogon                          = "logon"
	collectorThermalZone                    = "thermalzone"
	collectorTCP                            = "tcp"
	collectorIIS                            = "iis"
	collectorMSSQL                          = "mssql"
	collectorProcess                        = "process"
	collectorService                        = "service"
	collectorNetFrameworkCLRExceptions      = "netframework_clrexceptions"
	collectorNetFrameworkCLRInterop         = "netframework_clrinterop"
	collectorNetFrameworkCLRJIT             = "netframework_clrjit"
	collectorNetFrameworkCLRLoading         = "netframework_clrloading"
	collectorNetFrameworkCLRLocksAndThreads = "netframework_clrlocksandthreads"
	collectorNetFrameworkCLRMemory          = "netframework_clrmemory"
	collectorNetFrameworkCLRRemoting        = "netframework_clrremoting"
	collectorNetFrameworkCLRSecurity        = "netframework_clrsecurity"
	collectorExchange                       = "exchange"
	collectorHyperv                         = "hyperv"
)

func (c *Collector) collect() (map[string]int64, error) {
	pms, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	c.collectMetrics(mx, pms)

	if hasKey(mx, "os_visible_memory_bytes", "memory_available_bytes") {
		mx["memory_used_bytes"] = 0 +
			mx["os_visible_memory_bytes"] -
			mx["memory_available_bytes"]
	}
	if hasKey(mx, "os_paging_limit_bytes", "os_paging_free_bytes") {
		mx["os_paging_used_bytes"] = 0 +
			mx["os_paging_limit_bytes"] -
			mx["os_paging_free_bytes"]
	}
	if hasKey(mx, "os_visible_memory_bytes", "os_physical_memory_free_bytes") {
		mx["os_visible_memory_used_bytes"] = 0 +
			mx["os_visible_memory_bytes"] -
			mx["os_physical_memory_free_bytes"]
	}
	if hasKey(mx, "memory_commit_limit", "memory_committed_bytes") {
		mx["memory_not_committed_bytes"] = 0 +
			mx["memory_commit_limit"] -
			mx["memory_committed_bytes"]
	}
	if hasKey(mx, "memory_standby_cache_reserve_bytes", "memory_standby_cache_normal_priority_bytes", "memory_standby_cache_core_bytes") {
		mx["memory_standby_cache_total"] = 0 +
			mx["memory_standby_cache_reserve_bytes"] +
			mx["memory_standby_cache_normal_priority_bytes"] +
			mx["memory_standby_cache_core_bytes"]
	}
	if hasKey(mx, "memory_standby_cache_total", "memory_modified_page_list_bytes") {
		mx["memory_cache_total"] = 0 +
			mx["memory_standby_cache_total"] +
			mx["memory_modified_page_list_bytes"]
	}

	return mx, nil
}

func (c *Collector) collectMetrics(mx map[string]int64, pms prometheus.Series) {
	c.collectCollector(mx, pms)
	for _, pm := range pms.FindByName(metricCollectorSuccess) {
		if pm.Value == 0 {
			continue
		}

		switch pm.Labels.Get("collector") {
		case collectorCPU:
			c.collectCPU(mx, pms)
		case collectorMemory:
			c.collectMemory(mx, pms)
		case collectorNet:
			c.collectNet(mx, pms)
		case collectorLogicalDisk:
			c.collectLogicalDisk(mx, pms)
		case collectorOS:
			c.collectOS(mx, pms)
		case collectorSystem:
			c.collectSystem(mx, pms)
		case collectorLogon:
			c.collectLogon(mx, pms)
		case collectorThermalZone:
			c.collectThermalzone(mx, pms)
		case collectorTCP:
			c.collectTCP(mx, pms)
		case collectorProcess:
			c.collectProcess(mx, pms)
		case collectorService:
			c.collectService(mx, pms)
		case collectorIIS:
			c.collectIIS(mx, pms)
		case collectorMSSQL:
			c.collectMSSQL(mx, pms)
		case collectorAD:
			c.collectAD(mx, pms)
		case collectorADCS:
			c.collectADCS(mx, pms)
		case collectorADFS:
			c.collectADFS(mx, pms)
		case collectorNetFrameworkCLRExceptions:
			c.collectNetFrameworkCLRExceptions(mx, pms)
		case collectorNetFrameworkCLRInterop:
			c.collectNetFrameworkCLRInterop(mx, pms)
		case collectorNetFrameworkCLRJIT:
			c.collectNetFrameworkCLRJIT(mx, pms)
		case collectorNetFrameworkCLRLoading:
			c.collectNetFrameworkCLRLoading(mx, pms)
		case collectorNetFrameworkCLRLocksAndThreads:
			c.collectNetFrameworkCLRLocksAndThreads(mx, pms)
		case collectorNetFrameworkCLRMemory:
			c.collectNetFrameworkCLRMemory(mx, pms)
		case collectorNetFrameworkCLRRemoting:
			c.collectNetFrameworkCLRRemoting(mx, pms)
		case collectorNetFrameworkCLRSecurity:
			c.collectNetFrameworkCLRSecurity(mx, pms)
		case collectorExchange:
			c.collectExchange(mx, pms)
		case collectorHyperv:
			c.collectHyperv(mx, pms)
		}
	}
}

func hasKey(mx map[string]int64, key string, keys ...string) bool {
	_, ok := mx[key]
	switch len(keys) {
	case 0:
		return ok
	default:
		return ok && hasKey(mx, keys[0], keys[1:]...)
	}
}
