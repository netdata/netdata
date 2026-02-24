// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioClientSessionConnections = collectorapi.Priority + iota
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

var varnishCharts = collectorapi.Charts{
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

var backendChartsTmpl = collectorapi.Charts{
	backendDataTransferChartTmpl.Copy(),
}

var storageChartsTmpl = collectorapi.Charts{
	storageSpaceUsageChartTmpl.Copy(),
	storageAllocatedObjectsChartTmpl.Copy(),
}

// Client metrics
var (
	clientSessionConnectionsChart = collectorapi.Chart{
		ID:       "client_session_connections",
		Title:    "Client Session Connections",
		Fam:      "client connections",
		Units:    "connections/s",
		Ctx:      "varnish.client_session_connections",
		Type:     collectorapi.Line,
		Priority: prioClientSessionConnections,
		Dims: collectorapi.Dims{
			{ID: "MAIN.sess_conn", Name: "accepted", Algo: collectorapi.Incremental},
			{ID: "MAIN.sess_dropped", Name: "dropped", Algo: collectorapi.Incremental},
		},
	}

	clientRequestsChart = collectorapi.Chart{
		ID:       "client_requests",
		Title:    "Client Requests",
		Fam:      "client requests",
		Units:    "requests/s",
		Ctx:      "varnish.client_requests",
		Type:     collectorapi.Line,
		Priority: prioClientRequests,
		Dims: collectorapi.Dims{
			{ID: "MAIN.client_req", Name: "received", Algo: collectorapi.Incremental},
		},
	}
)

// Cache activity
var (
	cacheHitRatioTotalChart = collectorapi.Chart{
		ID:       "cache_hit_ratio_total",
		Title:    "Cache Hit Ratio Total",
		Fam:      "cache activity",
		Units:    "percent",
		Ctx:      "varnish.cache_hit_ratio_total",
		Type:     collectorapi.Stacked,
		Priority: prioCacheHitRatioTotal,
		Dims: collectorapi.Dims{
			{ID: "MAIN.cache_hit", Name: "hit", Algo: collectorapi.PercentOfAbsolute},
			{ID: "MAIN.cache_miss", Name: "miss", Algo: collectorapi.PercentOfAbsolute},
			{ID: "MAIN.cache_hitpass", Name: "hitpass", Algo: collectorapi.PercentOfAbsolute},
			{ID: "MAIN.cache_hitmiss", Name: "hitmiss", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	cacheHitRatioDeltaChart = collectorapi.Chart{
		ID:       "cache_hit_ratio_delta",
		Title:    "Cache Hit Ratio Current Poll",
		Fam:      "cache activity",
		Units:    "percent",
		Ctx:      "varnish.cache_hit_ratio_delta",
		Type:     collectorapi.Stacked,
		Priority: prioCacheHitRatioDelta,
		Dims: collectorapi.Dims{
			{ID: "MAIN.cache_hit", Name: "hit", Algo: collectorapi.PercentOfIncremental},
			{ID: "MAIN.cache_miss", Name: "miss", Algo: collectorapi.PercentOfIncremental},
			{ID: "MAIN.cache_hitpass", Name: "hitpass", Algo: collectorapi.PercentOfIncremental},
			{ID: "MAIN.cache_hitmiss", Name: "hitmiss", Algo: collectorapi.PercentOfIncremental},
		},
	}
	cachedObjectsExpiredChart = collectorapi.Chart{
		ID:       "cache_expired_objects",
		Title:    "Cache Expired Objects",
		Fam:      "cache activity",
		Units:    "objects/s",
		Ctx:      "varnish.cache_expired_objects",
		Type:     collectorapi.Line,
		Priority: prioCacheExpiredObjects,
		Dims: collectorapi.Dims{
			{ID: "MAIN.n_expired", Name: "expired", Algo: collectorapi.Incremental},
		},
	}
	cacheLRUActivityChart = collectorapi.Chart{
		ID:       "cache_lru_activity",
		Title:    "Cache LRU Activity",
		Fam:      "cache activity",
		Units:    "objects/s",
		Ctx:      "varnish.cache_lru_activity",
		Type:     collectorapi.Line,
		Priority: prioCacheLRUActivity,
		Dims: collectorapi.Dims{
			{ID: "MAIN.n_lru_nuked", Name: "nuked", Algo: collectorapi.Incremental},
			{ID: "MAIN.n_lru_moved", Name: "moved", Algo: collectorapi.Incremental},
		},
	}
)

// Threads
var (
	threadsTotalChart = collectorapi.Chart{
		ID:       "threads",
		Title:    "Threads In All Pools",
		Fam:      "threads",
		Units:    "threads",
		Ctx:      "varnish.threads",
		Type:     collectorapi.Line,
		Priority: prioThreadsTotal,
		Dims: collectorapi.Dims{
			{ID: "MAIN.threads", Name: "threads"},
		},
	}
	threadManagementActivityChart = collectorapi.Chart{
		ID:       "thread_management_activity",
		Title:    "Thread Management Activity",
		Fam:      "threads",
		Units:    "threads/s",
		Ctx:      "varnish.thread_management_activity",
		Type:     collectorapi.Line,
		Priority: prioThreadManagementActivity,
		Dims: collectorapi.Dims{
			{ID: "MAIN.threads_created", Name: "created", Algo: collectorapi.Incremental},
			{ID: "MAIN.threads_failed", Name: "failed", Algo: collectorapi.Incremental},
			{ID: "MAIN.threads_destroyed", Name: "destroyed", Algo: collectorapi.Incremental},
			{ID: "MAIN.threads_limited", Name: "limited", Algo: collectorapi.Incremental},
		},
	}
	threadQueueLenChart = collectorapi.Chart{
		ID:       "thread_queue_len",
		Title:    "Session Queue Length",
		Fam:      "threads",
		Units:    "requests",
		Ctx:      "varnish.thread_queue_len",
		Type:     collectorapi.Line,
		Priority: prioThreadQueueLen,
		Dims: collectorapi.Dims{
			{ID: "MAIN.thread_queue_len", Name: "queue_len"},
		},
	}
)

var (
	backendConnectionsChart = collectorapi.Chart{
		ID:       "backends_connections",
		Title:    "Backend Connections",
		Fam:      "backend connections",
		Units:    "connections/s",
		Ctx:      "varnish.backends_connections",
		Type:     collectorapi.Line,
		Priority: prioBackendsConnections,
		Dims: collectorapi.Dims{
			{ID: "MAIN.backend_conn", Name: "successful", Algo: collectorapi.Incremental},
			{ID: "MAIN.backend_unhealthy", Name: "unhealthy", Algo: collectorapi.Incremental},
			{ID: "MAIN.backend_busy", Name: "busy", Algo: collectorapi.Incremental},
			{ID: "MAIN.backend_fail", Name: "failed", Algo: collectorapi.Incremental},
			{ID: "MAIN.backend_reuse", Name: "reused", Algo: collectorapi.Incremental},
			{ID: "MAIN.backend_recycle", Name: "recycled", Algo: collectorapi.Incremental},
			{ID: "MAIN.backend_retry", Name: "retry", Algo: collectorapi.Incremental},
		},
	}
	backendRequestsChart = collectorapi.Chart{
		ID:       "backends_requests",
		Title:    "Backend Requests",
		Fam:      "backend requests",
		Units:    "requests/s",
		Ctx:      "varnish.backends_requests",
		Type:     collectorapi.Line,
		Priority: prioBackendsRequests,
		Dims: collectorapi.Dims{
			{ID: "MAIN.backend_req", Name: "sent", Algo: collectorapi.Incremental},
		},
	}
)

// ESI
var (
	esiParsingIssuesChart = collectorapi.Chart{
		ID:       "esi_parsing_issues",
		Title:    "ESI Parsing Issues",
		Fam:      "esi",
		Units:    "issues/s",
		Ctx:      "varnish.esi_parsing_issues",
		Type:     collectorapi.Line,
		Priority: prioEsiStatistics,
		Dims: collectorapi.Dims{
			{ID: "MAIN.esi_errors", Name: "errors", Algo: collectorapi.Incremental},
			{ID: "MAIN.esi_warnings", Name: "warnings", Algo: collectorapi.Incremental},
		},
	}
)

// Uptime
var (
	mgmtProcessUptimeChart = collectorapi.Chart{
		ID:       "mgmt_process_uptime",
		Title:    "Management Process Uptime",
		Fam:      "uptime",
		Units:    "seconds",
		Ctx:      "varnish.mgmt_process_uptime",
		Type:     collectorapi.Line,
		Priority: prioMgmtProcessUptime,
		Dims: collectorapi.Dims{
			{ID: "MGT.uptime", Name: "uptime"},
		},
	}
	childProcessUptimeChart = collectorapi.Chart{
		ID:       "child_process_uptime",
		Title:    "Child Process Uptime",
		Fam:      "uptime",
		Units:    "seconds",
		Ctx:      "varnish.child_process_uptime",
		Type:     collectorapi.Line,
		Priority: prioChildProcessUptime,
		Dims: collectorapi.Dims{
			{ID: "MAIN.uptime", Name: "uptime"},
		},
	}
)

var (
	backendDataTransferChartTmpl = collectorapi.Chart{
		ID:       "backend_%s_data_transfer",
		Title:    "Backend Data Transfer",
		Fam:      "backend traffic",
		Units:    "bytes/s",
		Ctx:      "varnish.backend_data_transfer",
		Type:     collectorapi.Area,
		Priority: prioBackendDataTransfer,
		Dims: collectorapi.Dims{
			{ID: "VBE.%s.bereq_hdrbytes", Name: "req_header", Algo: collectorapi.Incremental},
			{ID: "VBE.%s.bereq_bodybytes", Name: "req_body", Algo: collectorapi.Incremental},
			{ID: "VBE.%s.beresp_hdrbytes", Name: "resp_header", Algo: collectorapi.Incremental, Mul: -1},
			{ID: "VBE.%s.beresp_bodybytes", Name: "resp_body", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
)

var (
	storageSpaceUsageChartTmpl = collectorapi.Chart{
		ID:       "storage_%s_usage",
		Title:    "Storage Space Usage",
		Fam:      "storage usage",
		Units:    "bytes",
		Ctx:      "varnish.storage_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioStorageSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "%s.g_space", Name: "free"},
			{ID: "%s.g_bytes", Name: "used"},
		},
	}

	storageAllocatedObjectsChartTmpl = collectorapi.Chart{
		ID:       "storage_%s_allocated_objects",
		Title:    "Storage Allocated Objects",
		Fam:      "storage usage",
		Units:    "objects",
		Ctx:      "varnish.storage_allocated_objects",
		Type:     collectorapi.Line,
		Priority: prioStorageAllocatedObjects,
		Dims: collectorapi.Dims{
			{ID: "%s.g_alloc", Name: "allocated"},
		},
	}
)

func (c *Collector) addBackendCharts(fullName string) {
	charts := backendChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartID(fmt.Sprintf(chart.ID, fullName))
		chart.Labels = []collectorapi.Label{
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
		chart.Labels = []collectorapi.Label{
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
