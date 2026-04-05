// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var cacheCharts = collectorapi.Charts{
	{
		ID:    "servers_queries",
		Title: "Queries forwarded to the upstream servers",
		Units: "queries/s",
		Fam:   "servers",
		Ctx:   "dnsmasq.servers_queries",
		Dims: collectorapi.Dims{
			{ID: "queries", Name: "success", Algo: collectorapi.Incremental},
			{ID: "failed_queries", Name: "failed", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_performance",
		Title: "Cache performance",
		Units: "events/s",
		Fam:   "cache",
		Ctx:   "dnsmasq.cache_performance",
		Dims: collectorapi.Dims{
			{ID: "hits", Algo: collectorapi.Incremental},
			{ID: "misses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_operations",
		Title: "Cache operations",
		Units: "operations/s",
		Fam:   "cache",
		Ctx:   "dnsmasq.cache_operations",
		Dims: collectorapi.Dims{
			{ID: "insertions", Algo: collectorapi.Incremental},
			{ID: "evictions", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_size",
		Title: "Cache size",
		Units: "entries",
		Fam:   "cache",
		Ctx:   "dnsmasq.cache_size",
		Dims: collectorapi.Dims{
			{ID: "cachesize", Name: "size"},
		},
	},
}
