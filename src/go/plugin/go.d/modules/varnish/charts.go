// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioSessionConnections = module.Priority + iota
	prioClientRequests
	prioAllTimeHitRateChart
	prioCurrentPollHitRateChart
	prioCachedObjectsExpired
	prioCachedObjectsNuked
	prioThreadsTotal
	prioThreadsStatistics
	prioThreadsQueueLen
	prioBackendConnections
	prioBackendRequests
	prioEsiStatistics
	prioMemoryUsage
	prioUptime
	prioBackendResponseStatistics
	prioStorageUsage
	prioStorageAllocatedObjects
)

// const (
// 	byteToMiB = 1 << 20
// )

var varnishCharts = module.Charts{
	sessionConnectionsChart.Copy(),
	clientRequestsChart.Copy(),
	allTimeHitRateChart.Copy(),
	currentPollHitRateChart.Copy(),
	cachedObjectsExpiredChart.Copy(),
	cachedObjectsNukedChart.Copy(),
	threadsTotalChart.Copy(),
	threadsStatisticsChart.Copy(),
	threadsQueueLenChart.Copy(),
	backendConnectionsChart.Copy(),
	backendRequestsChart.Copy(),
	esiStatisticsChart.Copy(),
	// memoryUsageChart.Copy(),
	uptimeChart.Copy(),
}

var backendChartsTmpl = module.Charts{
	backendResponseStatisticsChartTmpl.Copy(),
}

var storageChartsTmpl = module.Charts{
	storageUsageChartTmpl.Copy(),
	storageAllocatedObjectsChartTmpl.Copy(),
}

var (
	sessionConnectionsChart = module.Chart{
		ID:       "session_connections",
		Title:    "Connections Statistics",
		Fam:      "client metrics",
		Units:    "connections/s",
		Ctx:      "varnish.session_connection",
		Type:     module.Line,
		Priority: prioSessionConnections,
		Dims: module.Dims{
			{ID: "sess_conn", Name: "accepted", Algo: module.Incremental},
			{ID: "sess_dropped", Name: "dropped", Algo: module.Incremental},
		},
	}

	clientRequestsChart = module.Chart{
		ID:       "client_requests",
		Title:    "Client Requests",
		Fam:      "client metrics",
		Units:    "requests/s",
		Ctx:      "varnish.client_requests",
		Type:     module.Line,
		Priority: prioClientRequests,
		Dims: module.Dims{
			{ID: "client_req", Name: "received", Algo: module.Incremental},
		},
	}

	allTimeHitRateChart = module.Chart{
		ID:       "all_time_hit_rate",
		Title:    "All History Hit Rate Ratio",
		Fam:      "cache performance",
		Units:    "percentage",
		Ctx:      "varnish.all_time_hit_rate",
		Type:     module.Stacked,
		Priority: prioAllTimeHitRateChart,
		Dims: module.Dims{
			{ID: "cache_hit", Name: "hit", Algo: module.PercentOfAbsolute},
			{ID: "cache_miss", Name: "miss", Algo: module.PercentOfAbsolute},
			{ID: "cache_hitpass", Name: "hitpass", Algo: module.PercentOfAbsolute},
		},
	}

	currentPollHitRateChart = module.Chart{
		ID:       "current_poll_hit_rate",
		Title:    "Current Poll Hit Rate Ratio",
		Fam:      "cache performance",
		Units:    "percentage",
		Ctx:      "varnish.current_poll_hit_rate",
		Type:     module.Stacked,
		Priority: prioCurrentPollHitRateChart,
		Dims: module.Dims{
			{ID: "cache_hit", Name: "hit", Algo: module.PercentOfIncremental},
			{ID: "cache_miss", Name: "miss", Algo: module.PercentOfIncremental},
			{ID: "cache_hitpass", Name: "hitpass", Algo: module.PercentOfIncremental},
		},
	}

	cachedObjectsExpiredChart = module.Chart{
		ID:       "cached_objects_expired",
		Title:    "Expired Objects",
		Fam:      "cache performance",
		Units:    "expired/s",
		Ctx:      "varnish.cached_objects_expired",
		Type:     module.Line,
		Priority: prioCachedObjectsExpired,
		Dims: module.Dims{
			{ID: "n_expired", Name: "objects", Algo: module.Incremental},
		},
	}

	cachedObjectsNukedChart = module.Chart{
		ID:       "cached_objects_nuked",
		Title:    "Least Recently Used Nuked Objects",
		Fam:      "cache performance",
		Units:    "nuked/s",
		Ctx:      "varnish.cached_objects_nuked",
		Type:     module.Line,
		Priority: prioCachedObjectsNuked,
		Dims: module.Dims{
			{ID: "n_lru_nuked", Name: "objects", Algo: module.Incremental},
		},
	}

	threadsTotalChart = module.Chart{
		ID:       "threads_total",
		Title:    "Number Of Threads In All Pools",
		Fam:      "thread related metrics",
		Units:    "number",
		Ctx:      "varnish.threads_total",
		Type:     module.Line,
		Priority: prioThreadsTotal,
		Dims: module.Dims{
			{ID: "threads"},
		},
	}

	threadsStatisticsChart = module.Chart{
		ID:       "threads_statistics",
		Title:    "Threads Statistics",
		Fam:      "thread related metrics",
		Units:    "threads/s",
		Ctx:      "varnish.threads_statistics",
		Type:     module.Line,
		Priority: prioThreadsStatistics,
		Dims: module.Dims{
			{ID: "threads_created", Name: "created", Algo: module.Incremental},
			{ID: "threads_failed", Name: "failed", Algo: module.Incremental},
			{ID: "threads_limited", Name: "limited", Algo: module.Incremental},
		},
	}

	threadsQueueLenChart = module.Chart{
		ID:       "threads_queue_len",
		Title:    "Current Queue Length",
		Fam:      "thread related metrics",
		Units:    "requests",
		Ctx:      "varnish.threads_queue_len",
		Type:     module.Line,
		Priority: prioThreadsQueueLen,
		Dims: module.Dims{
			{ID: "thread_queue_len", Name: "in queue"},
		},
	}

	backendConnectionsChart = module.Chart{
		ID:       "backend_connections",
		Title:    "Backend Connections Statistics",
		Fam:      "backend metrics",
		Units:    "connections/s",
		Ctx:      "varnish.backend_connections",
		Type:     module.Line,
		Priority: prioBackendConnections,
		Dims: module.Dims{
			{ID: "backend_conn", Name: "successful", Algo: module.Incremental},
			{ID: "backend_unhealthy", Name: "unhealthy", Algo: module.Incremental},
			{ID: "backend_busy", Name: "busy", Algo: module.Incremental},
			{ID: "backend_fail", Name: "failed", Algo: module.Incremental},
			{ID: "backend_reuse", Name: "reused", Algo: module.Incremental},
			{ID: "backend_recycle", Name: "recycled", Algo: module.Incremental},
			{ID: "backend_retry", Name: "retry", Algo: module.Incremental},
		},
	}

	backendRequestsChart = module.Chart{
		ID:       "backend_requests",
		Title:    "Requests To The Backend",
		Fam:      "backend metrics",
		Units:    "requests/s",
		Ctx:      "varnish.backend_requests",
		Type:     module.Line,
		Priority: prioBackendRequests,
		Dims: module.Dims{
			{ID: "backend_req", Name: "sent", Algo: module.Incremental},
		},
	}

	esiStatisticsChart = module.Chart{
		ID:       "esi_statistics",
		Title:    "ESI Statistics",
		Fam:      "esi related metrics",
		Units:    "problems/s",
		Ctx:      "varnish.esi_statistics",
		Type:     module.Line,
		Priority: prioEsiStatistics,
		Dims: module.Dims{
			{ID: "esi_errors", Name: "errors", Algo: module.Incremental},
			{ID: "esi_warnings", Name: "warnings", Algo: module.Incremental},
		},
	}

	// metrics not existing anymore
	// memoryUsageChart = module.Chart{
	// 	ID:       "memory_usage",
	// 	Title:    "Memory Usage",
	// 	Fam:      "memory usage",
	// 	Units:    "MiB",
	// 	Ctx:      "varnish.memory_usage",
	// 	Type:     module.Stacked,
	// 	Priority: prioMemoryUsage,
	// 	Dims: module.Dims{
	// 		{ID: "memory_free", Name: "free", Div: byteToMiB},
	// 		{ID: "memory_allocated", Name: "allocated", Div: byteToMiB},
	// 	},
	// }

	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Fam:      "uptime",
		Units:    "seconds",
		Ctx:      "varnish.uptime",
		Type:     module.Line,
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}

	backendResponseStatisticsChartTmpl = module.Chart{
		ID:       "%s_response_statistics",
		Title:    "Backend Response Statistics",
		Fam:      "backend response statistics",
		Units:    "kilobits/s",
		Ctx:      "varnish.backend",
		Type:     module.Area,
		Priority: prioBackendResponseStatistics,
		Dims: module.Dims{
			{ID: "%s_beresp_hdrbytes", Name: "header", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "%s_beresp_bodybytes", Name: "body", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	}

	storageUsageChartTmpl = module.Chart{
		ID:       "storage_%s_usage",
		Title:    "Storage Usage",
		Fam:      "storage usage",
		Units:    "KiB",
		Ctx:      "varnish.storage_usage",
		Type:     module.Stacked,
		Priority: prioStorageUsage,
		Dims: module.Dims{
			{ID: "%s_g_space", Name: "free", Div: 1 << 10},
			{ID: "%s_g_bytes", Name: "allocated", Div: 1 << 10},
		},
	}

	storageAllocatedObjectsChartTmpl = module.Chart{
		ID:       "storage_%s_alloc_objs",
		Title:    "Storage Allocated Objects",
		Fam:      "storage usage",
		Units:    "objects",
		Ctx:      "varnish.storage_alloc_objs",
		Type:     module.Line,
		Priority: prioStorageAllocatedObjects,
		Dims: module.Dims{
			{ID: "%s_g_alloc", Name: "allocated"},
		},
	}
)

func (v *Varnish) addBackendCharts(name string) {
	charts := backendChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := v.Charts().Add(*charts...); err != nil {
		v.Warning(err)
	}

}

func (v *Varnish) addStorageCharts(name string) {
	charts := storageChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := v.Charts().Add(*charts...); err != nil {
		v.Warning(err)
	}

}

func (v *Varnish) removeBackendCharts(name string) {
	px := fmt.Sprintf("%s_response_statistics", name)
	for _, chart := range *v.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (v *Varnish) removeStorageCharts(name string) {
	px := fmt.Sprintf("storage_%s", name)
	for _, chart := range *v.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
