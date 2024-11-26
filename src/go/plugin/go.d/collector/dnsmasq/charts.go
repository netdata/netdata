// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var cacheCharts = module.Charts{
	{
		ID:    "servers_queries",
		Title: "Queries forwarded to the upstream servers",
		Units: "queries/s",
		Fam:   "servers",
		Ctx:   "dnsmasq.servers_queries",
		Dims: module.Dims{
			{ID: "queries", Name: "success", Algo: module.Incremental},
			{ID: "failed_queries", Name: "failed", Algo: module.Incremental},
		},
	},
	{
		ID:    "cache_performance",
		Title: "Cache performance",
		Units: "events/s",
		Fam:   "cache",
		Ctx:   "dnsmasq.cache_performance",
		Dims: module.Dims{
			{ID: "hits", Algo: module.Incremental},
			{ID: "misses", Algo: module.Incremental},
		},
	},
	{
		ID:    "cache_operations",
		Title: "Cache operations",
		Units: "operations/s",
		Fam:   "cache",
		Ctx:   "dnsmasq.cache_operations",
		Dims: module.Dims{
			{ID: "insertions", Algo: module.Incremental},
			{ID: "evictions", Algo: module.Incremental},
		},
	},
	{
		ID:    "cache_size",
		Title: "Cache size",
		Units: "entries",
		Fam:   "cache",
		Ctx:   "dnsmasq.cache_size",
		Dims: module.Dims{
			{ID: "cachesize", Name: "size"},
		},
	},
}
