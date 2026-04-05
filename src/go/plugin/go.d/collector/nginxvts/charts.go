// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var mainCharts = collectorapi.Charts{
	{
		ID:    "requests",
		Title: "Total requests",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "nginxvts.requests_total",
		Dims: collectorapi.Dims{
			{ID: "connections_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "active_connections",
		Title: "Active connections",
		Units: "connections",
		Fam:   "connections",
		Ctx:   "nginxvts.active_connections",
		Dims: collectorapi.Dims{
			{ID: "connections_active", Name: "active"},
		},
	},
	{
		ID:    "connections",
		Title: "Total connections",
		Units: "connections/s",
		Fam:   "connections",
		Ctx:   "nginxvts.connections_total",
		Dims: collectorapi.Dims{
			{ID: "connections_reading", Name: "reading", Algo: collectorapi.Incremental},
			{ID: "connections_writing", Name: "writing", Algo: collectorapi.Incremental},
			{ID: "connections_waiting", Name: "waiting", Algo: collectorapi.Incremental},
			{ID: "connections_accepted", Name: "accepted", Algo: collectorapi.Incremental},
			{ID: "connections_handled", Name: "handled", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "uptime",
		Title: "Uptime",
		Units: "seconds",
		Fam:   "uptime",
		Ctx:   "nginxvts.uptime",
		Dims: collectorapi.Dims{
			{ID: "uptime", Name: "uptime"},
		},
	},
}
var sharedZonesCharts = collectorapi.Charts{
	{
		ID:    "shared_memory_size",
		Title: "Shared memory size",
		Units: "bytes",
		Fam:   "shared memory",
		Ctx:   "nginxvts.shm_usage",
		Dims: collectorapi.Dims{
			{ID: "sharedzones_maxsize", Name: "max"},
			{ID: "sharedzones_usedsize", Name: "used"},
		},
	},
	{
		ID:    "shared_memory_used_node",
		Title: "Number of node using shared memory",
		Units: "nodes",
		Fam:   "shared memory",
		Ctx:   "nginxvts.shm_used_node",
		Dims: collectorapi.Dims{
			{ID: "sharedzones_usednode", Name: "used"},
		},
	},
}

var serverZonesCharts = collectorapi.Charts{
	{
		ID:    "server_requests_total",
		Title: "Total number of client requests",
		Units: "requests/s",
		Fam:   "serverzones",
		Ctx:   "nginxvts.server_requests_total",
		Dims: collectorapi.Dims{
			{ID: "total_requestcounter", Name: "requests", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "server_responses_total",
		Title: "Total number of responses by code class",
		Units: "responses/s",
		Fam:   "serverzones",
		Ctx:   "nginxvts.server_responses_total",
		Dims: collectorapi.Dims{
			{ID: "total_responses_1xx", Name: "1xx", Algo: collectorapi.Incremental},
			{ID: "total_responses_2xx", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "total_responses_3xx", Name: "3xx", Algo: collectorapi.Incremental},
			{ID: "total_responses_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "total_responses_5xx", Name: "5xx", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "server_traffic_total",
		Title: "Total amount of data transferred to and from the server",
		Units: "bytes/s",
		Fam:   "serverzones",
		Ctx:   "nginxvts.server_traffic_total",
		Dims: collectorapi.Dims{
			{ID: "total_inbytes", Name: "in", Algo: collectorapi.Incremental},
			{ID: "total_outbytes", Name: "out", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "server_cache_total",
		Title: "Total server cache",
		Units: "events/s",
		Fam:   "serverzones",
		Ctx:   "nginxvts.server_cache_total",
		Dims: collectorapi.Dims{
			{ID: "total_cache_miss", Name: "miss", Algo: collectorapi.Incremental},
			{ID: "total_cache_bypass", Name: "bypass", Algo: collectorapi.Incremental},
			{ID: "total_cache_expired", Name: "expired", Algo: collectorapi.Incremental},
			{ID: "total_cache_stale", Name: "stale", Algo: collectorapi.Incremental},
			{ID: "total_cache_updating", Name: "updating", Algo: collectorapi.Incremental},
			{ID: "total_cache_revalidated", Name: "revalidated", Algo: collectorapi.Incremental},
			{ID: "total_cache_hit", Name: "hit", Algo: collectorapi.Incremental},
			{ID: "total_cache_scarce", Name: "scarce", Algo: collectorapi.Incremental},
		},
	},
}
