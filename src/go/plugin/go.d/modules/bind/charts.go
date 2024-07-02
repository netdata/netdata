// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type (
	// Charts is an alias for module.Charts.
	Charts = module.Charts
	// Chart is an alias for module.Chart.
	Chart = module.Chart
	// Dims is an alias for module.Dims.
	Dims = module.Dims
	// Dim is an alias for module.Dim.
	Dim = module.Dim
)

const (
	// TODO: add to orchestrator module
	basePriority = 70000

	keyReceivedRequests    = "received_requests"
	keyQueriesSuccess      = "queries_success"
	keyRecursiveClients    = "recursive_clients"
	keyProtocolsQueries    = "protocols_queries"
	keyQueriesAnalysis     = "queries_analysis"
	keyReceivedUpdates     = "received_updates"
	keyQueryFailures       = "query_failures"
	keyQueryFailuresDetail = "query_failures_detail"
	keyNSStats             = "nsstats"
	keyInOpCodes           = "in_opcodes"
	keyInQTypes            = "in_qtypes"
	keyInSockStats         = "in_sockstats"

	keyResolverStats     = "view_resolver_stats_%s"
	keyResolverRTT       = "view_resolver_rtt_%s"
	keyResolverInQTypes  = "view_resolver_qtypes_%s"
	keyResolverCacheHits = "view_resolver_cachehits_%s"
	keyResolverNumFetch  = "view_resolver_numfetch_%s"
)

var charts = map[string]Chart{
	keyRecursiveClients: {
		ID:       keyRecursiveClients,
		Title:    "Global Recursive Clients",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "bind.recursive_clients",
		Priority: basePriority + 1,
	},
	keyReceivedRequests: {
		ID:       keyReceivedRequests,
		Title:    "Global Received Requests by IP version",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "bind.requests",
		Type:     module.Stacked,
		Priority: basePriority + 2,
	},
	keyQueriesSuccess: {
		ID:       keyQueriesSuccess,
		Title:    "Global Successful Queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "bind.queries_success",
		Priority: basePriority + 3,
	},
	keyProtocolsQueries: {
		ID:       keyProtocolsQueries,
		Title:    "Global Queries by IP Protocol",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "bind.protocol_queries",
		Type:     module.Stacked,
		Priority: basePriority + 4,
	},
	keyQueriesAnalysis: {
		ID:       keyQueriesAnalysis,
		Title:    "Global Queries Analysis",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "bind.global_queries",
		Type:     module.Stacked,
		Priority: basePriority + 5,
	},
	keyReceivedUpdates: {
		ID:       keyReceivedUpdates,
		Title:    "Global Received Updates",
		Units:    "updates/s",
		Fam:      "updates",
		Ctx:      "bind.global_updates",
		Type:     module.Stacked,
		Priority: basePriority + 6,
	},
	keyQueryFailures: {
		ID:       keyQueryFailures,
		Title:    "Global Query Failures",
		Units:    "failures/s",
		Fam:      "failures",
		Ctx:      "bind.global_failures",
		Priority: basePriority + 7,
	},
	keyQueryFailuresDetail: {
		ID:       keyQueryFailuresDetail,
		Title:    "Global Query Failures Analysis",
		Units:    "failures/s",
		Fam:      "failures",
		Ctx:      "bind.global_failures_detail",
		Type:     module.Stacked,
		Priority: basePriority + 8,
	},
	keyNSStats: {
		ID:       keyNSStats,
		Title:    "Global Server Statistics",
		Units:    "operations/s",
		Fam:      "other",
		Ctx:      "bind.nsstats",
		Priority: basePriority + 9,
	},
	keyInOpCodes: {
		ID:       keyInOpCodes,
		Title:    "Incoming Requests by OpCode",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "bind.in_opcodes",
		Type:     module.Stacked,
		Priority: basePriority + 10,
	},
	keyInQTypes: {
		ID:       keyInQTypes,
		Title:    "Incoming Requests by Query Type",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "bind.in_qtypes",
		Type:     module.Stacked,
		Priority: basePriority + 11,
	},
	keyInSockStats: {
		ID:       keyInSockStats,
		Title:    "Socket Statistics",
		Units:    "operations/s",
		Fam:      "sockets",
		Ctx:      "bind.in_sockstats",
		Priority: basePriority + 12,
	},

	keyResolverRTT: {
		ID:       keyResolverRTT,
		Title:    "Resolver Round Trip Time",
		Units:    "queries/s",
		Fam:      "view %s",
		Ctx:      "bind.resolver_rtt",
		Type:     module.Stacked,
		Priority: basePriority + 22,
	},
	keyResolverStats: {
		ID:       keyResolverStats,
		Title:    "Resolver Statistics",
		Units:    "operations/s",
		Fam:      "view %s",
		Ctx:      "bind.resolver_stats",
		Priority: basePriority + 23,
	},
	keyResolverInQTypes: {
		ID:       keyResolverInQTypes,
		Title:    "Resolver Requests by Query Type",
		Units:    "requests/s",
		Fam:      "view %s",
		Ctx:      "bind.resolver_qtypes",
		Type:     module.Stacked,
		Priority: basePriority + 24,
	},
	keyResolverNumFetch: {
		ID:       keyResolverNumFetch,
		Title:    "Resolver Active Queries",
		Units:    "queries",
		Fam:      "view %s",
		Ctx:      "bind.resolver_active_queries",
		Priority: basePriority + 25,
	},
	keyResolverCacheHits: {
		ID:       keyResolverCacheHits,
		Title:    "Resolver Cache Hits",
		Units:    "operations/s",
		Fam:      "view %s",
		Ctx:      "bind.resolver_cachehits",
		Type:     module.Area,
		Priority: basePriority + 26,
		Dims: Dims{
			{ID: "%s_CacheHits", Name: "hits", Algo: module.Incremental},
			{ID: "%s_CacheMisses", Name: "misses", Algo: module.Incremental, Mul: -1},
		},
	},
}
