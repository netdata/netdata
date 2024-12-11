// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClientSessionConnections = module.Priority + iota
	prioClientRequests

	prioBackendsConnections
	prioBackendsRequests
	prioBackendDataTransfer

	prioCacheHitRatioTotal
	prioCacheHitRatioDelta

	prioCacheExpiredObjects
	prioCacheLRUActivity

	prioThreadsTotal
	prioThreadManagementActivity
	prioThreadQueueLen

	prioEsiStatistics

	prioStorageSpaceUsage
	prioStorageAllocatedObjects

	prioMgmtProcessUptime
	prioChildProcessUptime
)

var varnishCharts = module.Charts{
	clientSessionConnectionsChart.Copy(),
	clientRequestsChart.Copy(),

	backendConnectionsChart.Copy(),
	backendRequestsChart.Copy(),

	cacheHitRatioTotalChart.Copy(),
	cacheHitRatioDeltaChart.Copy(),
	cachedObjectsExpiredChart.Copy(),
	cacheLRUActivityChart.Copy(),

	threadsTotalChart.Copy(),
	threadManagementActivityChart.Copy(),
	threadQueueLenChart.Copy(),

	esiParsingIssuesChart.Copy(),

	mgmtProcessUptimeChart.Copy(),
	childProcessUptimeChart.Copy(),
}

var backendChartsTmpl = module.Charts{
	backendDataTransferChartTmpl.Copy(),
}

var storageChartsTmpl = module.Charts{
	storageSpaceUsageChartTmpl.Copy(),
	storageAllocatedObjectsChartTmpl.Copy(),
}

// Client metrics
var (
	clientSessionConnectionsChart = module.Chart{
		ID:       "client_session_connections",
		Title:    "Client Session Connections",
		Fam:      "client connections",
		Units:    "connections/s",
		Ctx:      "varnish.client_session_connections",
		Type:     module.Line,
		Priority: prioClientSessionConnections,
		Dims: module.Dims{
			{ID: "MAIN.sess_conn", Name: "accepted", Algo: module.Incremental},
			{ID: "MAIN.sess_dropped", Name: "dropped", Algo: module.Incremental},
		},
	}

	clientRequestsChart = module.Chart{
		ID:       "client_requests",
		Title:    "Client Requests",
		Fam:      "client requests",
		Units:    "requests/s",
		Ctx:      "varnish.client_requests",
		Type:     module.Line,
		Priority: prioClientRequests,
		Dims: module.Dims{
			{ID: "MAIN.client_req", Name: "received", Algo: module.Incremental},
		},
	}
)

// Cache activity
var (
	cacheHitRatioTotalChart = module.Chart{
		ID:       "cache_hit_ratio_total",
		Title:    "Cache Hit Ratio Total",
		Fam:      "cache activity",
		Units:    "percent",
		Ctx:      "varnish.cache_hit_ratio_total",
		Type:     module.Stacked,
		Priority: prioCacheHitRatioTotal,
		Dims: module.Dims{
			{ID: "MAIN.cache_hit", Name: "hit", Algo: module.PercentOfAbsolute},
			{ID: "MAIN.cache_miss", Name: "miss", Algo: module.PercentOfAbsolute},
			{ID: "MAIN.cache_hitpass", Name: "hitpass", Algo: module.PercentOfAbsolute},
			{ID: "MAIN.cache_hitmiss", Name: "hitmiss", Algo: module.PercentOfAbsolute},
		},
	}
	cacheHitRatioDeltaChart = module.Chart{
		ID:       "cache_hit_ratio_delta",
		Title:    "Cache Hit Ratio Current Poll",
		Fam:      "cache activity",
		Units:    "percent",
		Ctx:      "varnish.cache_hit_ratio_delta",
		Type:     module.Stacked,
		Priority: prioCacheHitRatioDelta,
		Dims: module.Dims{
			{ID: "MAIN.cache_hit", Name: "hit", Algo: module.PercentOfIncremental},
			{ID: "MAIN.cache_miss", Name: "miss", Algo: module.PercentOfIncremental},
			{ID: "MAIN.cache_hitpass", Name: "hitpass", Algo: module.PercentOfIncremental},
			{ID: "MAIN.cache_hitmiss", Name: "hitmiss", Algo: module.PercentOfIncremental},
		},
	}
	cachedObjectsExpiredChart = module.Chart{
		ID:       "cache_expired_objects",
		Title:    "Cache Expired Objects",
		Fam:      "cache activity",
		Units:    "objects/s",
		Ctx:      "varnish.cache_expired_objects",
		Type:     module.Line,
		Priority: prioCacheExpiredObjects,
		Dims: module.Dims{
			{ID: "MAIN.n_expired", Name: "expired", Algo: module.Incremental},
		},
	}
	cacheLRUActivityChart = module.Chart{
		ID:       "cache_lru_activity",
		Title:    "Cache LRU Activity",
		Fam:      "cache activity",
		Units:    "objects/s",
		Ctx:      "varnish.cache_lru_activity",
		Type:     module.Line,
		Priority: prioCacheLRUActivity,
		Dims: module.Dims{
			{ID: "MAIN.n_lru_nuked", Name: "nuked", Algo: module.Incremental},
			{ID: "MAIN.n_lru_moved", Name: "moved", Algo: module.Incremental},
		},
	}
)

// Threads
var (
	threadsTotalChart = module.Chart{
		ID:       "threads",
		Title:    "Threads In All Pools",
		Fam:      "threads",
		Units:    "threads",
		Ctx:      "varnish.threads",
		Type:     module.Line,
		Priority: prioThreadsTotal,
		Dims: module.Dims{
			{ID: "MAIN.threads", Name: "threads"},
		},
	}
	threadManagementActivityChart = module.Chart{
		ID:       "thread_management_activity",
		Title:    "Thread Management Activity",
		Fam:      "threads",
		Units:    "threads/s",
		Ctx:      "varnish.thread_management_activity",
		Type:     module.Line,
		Priority: prioThreadManagementActivity,
		Dims: module.Dims{
			{ID: "MAIN.threads_created", Name: "created", Algo: module.Incremental},
			{ID: "MAIN.threads_failed", Name: "failed", Algo: module.Incremental},
			{ID: "MAIN.threads_destroyed", Name: "destroyed", Algo: module.Incremental},
			{ID: "MAIN.threads_limited", Name: "limited", Algo: module.Incremental},
		},
	}
	threadQueueLenChart = module.Chart{
		ID:       "thread_queue_len",
		Title:    "Session Queue Length",
		Fam:      "threads",
		Units:    "requests",
		Ctx:      "varnish.thread_queue_len",
		Type:     module.Line,
		Priority: prioThreadQueueLen,
		Dims: module.Dims{
			{ID: "MAIN.thread_queue_len", Name: "queue_len"},
		},
	}
)

var (
	backendConnectionsChart = module.Chart{
		ID:       "backends_connections",
		Title:    "Backend Connections",
		Fam:      "backend connections",
		Units:    "connections/s",
		Ctx:      "varnish.backends_connections",
		Type:     module.Line,
		Priority: prioBackendsConnections,
		Dims: module.Dims{
			{ID: "MAIN.backend_conn", Name: "successful", Algo: module.Incremental},
			{ID: "MAIN.backend_unhealthy", Name: "unhealthy", Algo: module.Incremental},
			{ID: "MAIN.backend_busy", Name: "busy", Algo: module.Incremental},
			{ID: "MAIN.backend_fail", Name: "failed", Algo: module.Incremental},
			{ID: "MAIN.backend_reuse", Name: "reused", Algo: module.Incremental},
			{ID: "MAIN.backend_recycle", Name: "recycled", Algo: module.Incremental},
			{ID: "MAIN.backend_retry", Name: "retry", Algo: module.Incremental},
		},
	}
	backendRequestsChart = module.Chart{
		ID:       "backends_requests",
		Title:    "Backend Requests",
		Fam:      "backend requests",
		Units:    "requests/s",
		Ctx:      "varnish.backends_requests",
		Type:     module.Line,
		Priority: prioBackendsRequests,
		Dims: module.Dims{
			{ID: "MAIN.backend_req", Name: "sent", Algo: module.Incremental},
		},
	}
)

// ESI
var (
	esiParsingIssuesChart = module.Chart{
		ID:       "esi_parsing_issues",
		Title:    "ESI Parsing Issues",
		Fam:      "esi",
		Units:    "issues/s",
		Ctx:      "varnish.esi_parsing_issues",
		Type:     module.Line,
		Priority: prioEsiStatistics,
		Dims: module.Dims{
			{ID: "MAIN.esi_errors", Name: "errors", Algo: module.Incremental},
			{ID: "MAIN.esi_warnings", Name: "warnings", Algo: module.Incremental},
		},
	}
)

// Uptime
var (
	mgmtProcessUptimeChart = module.Chart{
		ID:       "mgmt_process_uptime",
		Title:    "Management Process Uptime",
		Fam:      "uptime",
		Units:    "seconds",
		Ctx:      "varnish.mgmt_process_uptime",
		Type:     module.Line,
		Priority: prioMgmtProcessUptime,
		Dims: module.Dims{
			{ID: "MGT.uptime", Name: "uptime"},
		},
	}
	childProcessUptimeChart = module.Chart{
		ID:       "child_process_uptime",
		Title:    "Child Process Uptime",
		Fam:      "uptime",
		Units:    "seconds",
		Ctx:      "varnish.child_process_uptime",
		Type:     module.Line,
		Priority: prioChildProcessUptime,
		Dims: module.Dims{
			{ID: "MAIN.uptime", Name: "uptime"},
		},
	}
)

var (
	backendDataTransferChartTmpl = module.Chart{
		ID:       "backend_%s_data_transfer",
		Title:    "Backend Data Transfer",
		Fam:      "backend traffic",
		Units:    "bytes/s",
		Ctx:      "varnish.backend_data_transfer",
		Type:     module.Area,
		Priority: prioBackendDataTransfer,
		Dims: module.Dims{
			{ID: "VBE.%s.bereq_hdrbytes", Name: "req_header", Algo: module.Incremental},
			{ID: "VBE.%s.bereq_bodybytes", Name: "req_body", Algo: module.Incremental},
			{ID: "VBE.%s.beresp_hdrbytes", Name: "resp_header", Algo: module.Incremental, Mul: -1},
			{ID: "VBE.%s.beresp_bodybytes", Name: "resp_body", Algo: module.Incremental, Mul: -1},
		},
	}
)

var (
	storageSpaceUsageChartTmpl = module.Chart{
		ID:       "storage_%s_usage",
		Title:    "Storage Space Usage",
		Fam:      "storage usage",
		Units:    "bytes",
		Ctx:      "varnish.storage_space_usage",
		Type:     module.Stacked,
		Priority: prioStorageSpaceUsage,
		Dims: module.Dims{
			{ID: "%s.g_space", Name: "free"},
			{ID: "%s.g_bytes", Name: "used"},
		},
	}

	storageAllocatedObjectsChartTmpl = module.Chart{
		ID:       "storage_%s_allocated_objects",
		Title:    "Storage Allocated Objects",
		Fam:      "storage usage",
		Units:    "objects",
		Ctx:      "varnish.storage_allocated_objects",
		Type:     module.Line,
		Priority: prioStorageAllocatedObjects,
		Dims: module.Dims{
			{ID: "%s.g_alloc", Name: "allocated"},
		},
	}
)

func (c *Collector) addBackendCharts(fullName string) {
	charts := backendChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartID(fmt.Sprintf(chart.ID, fullName))
		chart.Labels = []module.Label{
			{Key: "backend", Value: fullName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, fullName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}

}

func (c *Collector) addStorageCharts(name string) {
	charts := storageChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartID(fmt.Sprintf(chart.ID, name))
		chart.Labels = []module.Label{
			{Key: "storage", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}

}

func (c *Collector) removeBackendCharts(name string) {
	px := fmt.Sprintf("backend_%s_", name)
	c.removeCharts(cleanChartID(px))
}

func (c *Collector) removeStorageCharts(name string) {
	px := fmt.Sprintf("storage_%s_", name)
	c.removeCharts(cleanChartID(px))
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanChartID(id string) string {
	id = strings.ReplaceAll(id, ".", "_")
	return strings.ToLower(id)
}
