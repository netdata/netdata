// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricMemAvailBytes                      = "windows_memory_available_bytes"
	metricMemCacheFaultsTotal                = "windows_memory_cache_faults_total"
	metricMemCommitLimit                     = "windows_memory_commit_limit"
	metricMemCommittedBytes                  = "windows_memory_committed_bytes"
	metricMemModifiedPageListBytes           = "windows_memory_modified_page_list_bytes"
	metricMemPageFaultsTotal                 = "windows_memory_page_faults_total"
	metricMemSwapPageReadsTotal              = "windows_memory_swap_page_reads_total"
	metricMemSwapPagesReadTotal              = "windows_memory_swap_pages_read_total"
	metricMemSwapPagesWrittenTotal           = "windows_memory_swap_pages_written_total"
	metricMemSwapPageWritesTotal             = "windows_memory_swap_page_writes_total"
	metricMemPoolNonPagedBytesTotal          = "windows_memory_pool_nonpaged_bytes"
	metricMemPoolPagedBytes                  = "windows_memory_pool_paged_bytes"
	metricMemStandbyCacheCoreBytes           = "windows_memory_standby_cache_core_bytes"
	metricMemStandbyCacheNormalPriorityBytes = "windows_memory_standby_cache_normal_priority_bytes"
	metricMemStandbyCacheReserveBytes        = "windows_memory_standby_cache_reserve_bytes"
)

func (w *Windows) collectMemory(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorMemory] {
		w.cache.collection[collectorMemory] = true
		w.addMemoryCharts()
	}

	if pm := pms.FindByName(metricMemAvailBytes); pm.Len() > 0 {
		mx["memory_available_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemCacheFaultsTotal); pm.Len() > 0 {
		mx["memory_cache_faults_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemCommitLimit); pm.Len() > 0 {
		mx["memory_commit_limit"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemCommittedBytes); pm.Len() > 0 {
		mx["memory_committed_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemModifiedPageListBytes); pm.Len() > 0 {
		mx["memory_modified_page_list_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemPageFaultsTotal); pm.Len() > 0 {
		mx["memory_page_faults_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemSwapPageReadsTotal); pm.Len() > 0 {
		mx["memory_swap_page_reads_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemSwapPagesReadTotal); pm.Len() > 0 {
		mx["memory_swap_pages_read_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemSwapPagesWrittenTotal); pm.Len() > 0 {
		mx["memory_swap_pages_written_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemSwapPageWritesTotal); pm.Len() > 0 {
		mx["memory_swap_page_writes_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemPoolNonPagedBytesTotal); pm.Len() > 0 {
		mx["memory_pool_nonpaged_bytes_total"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemPoolPagedBytes); pm.Len() > 0 {
		mx["memory_pool_paged_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemStandbyCacheCoreBytes); pm.Len() > 0 {
		mx["memory_standby_cache_core_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemStandbyCacheNormalPriorityBytes); pm.Len() > 0 {
		mx["memory_standby_cache_normal_priority_bytes"] = int64(pm.Max())
	}
	if pm := pms.FindByName(metricMemStandbyCacheReserveBytes); pm.Len() > 0 {
		mx["memory_standby_cache_reserve_bytes"] = int64(pm.Max())
	}
}
