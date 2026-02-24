// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var charts = collectorapi.Charts{
	{
		ID:    "queries",
		Title: "Client queries received",
		Units: "queries/s",
		Fam:   "queries",
		Ctx:   "dnsdist.queries",
		Dims: collectorapi.Dims{
			{ID: "queries", Name: "all", Algo: collectorapi.Incremental},
			{ID: "rdqueries", Name: "recursive", Algo: collectorapi.Incremental},
			{ID: "empty-queries", Name: "empty", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "queries_dropped",
		Title: "Client queries dropped",
		Units: "queries/s",
		Fam:   "queries",
		Ctx:   "dnsdist.queries_dropped",
		Dims: collectorapi.Dims{
			{ID: "rule-drop", Name: "rule drop", Algo: collectorapi.Incremental},
			{ID: "dyn-blocked", Name: "dynamic blocked", Algo: collectorapi.Incremental},
			{ID: "no-policy", Name: "no policy", Algo: collectorapi.Incremental},
			{ID: "noncompliant-queries", Name: "non queries", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "packets_dropped",
		Title: "Packets dropped",
		Units: "packets/s",
		Fam:   "packets",
		Ctx:   "dnsdist.packets_dropped",
		Dims: collectorapi.Dims{
			{ID: "acl-drops", Name: "acl", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "answers",
		Title: "Answers statistics",
		Units: "answers/s",
		Fam:   "answers",
		Ctx:   "dnsdist.answers",
		Dims: collectorapi.Dims{
			{ID: "self-answered", Name: "self answered", Algo: collectorapi.Incremental},
			{ID: "rule-nxdomain", Name: "nxdomain", Algo: collectorapi.Incremental, Mul: -1},
			{ID: "rule-refused", Name: "refused", Algo: collectorapi.Incremental, Mul: -1},
			{ID: "trunc-failures", Name: "trunc failures", Algo: collectorapi.Incremental, Mul: -1},
		},
	},
	{
		ID:    "backend_responses",
		Title: "Backend responses",
		Units: "responses/s",
		Fam:   "backends",
		Ctx:   "dnsdist.backend_responses",
		Dims: collectorapi.Dims{
			{ID: "responses", Name: "responses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "backend_commerrors",
		Title: "Backend communication errors",
		Units: "errors/s",
		Fam:   "backends",
		Ctx:   "dnsdist.backend_commerrors",
		Dims: collectorapi.Dims{
			{ID: "downstream-send-errors", Name: "send errors", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "backend_errors",
		Title: "Backend error responses",
		Units: "responses/s",
		Fam:   "backends",
		Ctx:   "dnsdist.backend_errors",
		Dims: collectorapi.Dims{
			{ID: "downstream-timeouts", Name: "timeouts", Algo: collectorapi.Incremental},
			{ID: "servfail-responses", Name: "servfail", Algo: collectorapi.Incremental},
			{ID: "noncompliant-responses", Name: "non compliant", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache",
		Title: "Cache performance",
		Units: "answers/s",
		Fam:   "cache",
		Ctx:   "dnsdist.cache",
		Dims: collectorapi.Dims{
			{ID: "cache-hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "cache-misses", Name: "misses", Algo: collectorapi.Incremental, Mul: -1},
		},
	},
	{
		ID:    "servercpu",
		Title: "DNSdist server CPU utilization",
		Units: "ms/s",
		Fam:   "server",
		Ctx:   "dnsdist.servercpu",
		Type:  collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "cpu-sys-msec", Name: "system state", Algo: collectorapi.Incremental},
			{ID: "cpu-user-msec", Name: "user state", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "servermem",
		Title: "DNSdist server memory utilization",
		Units: "MiB",
		Fam:   "server",
		Ctx:   "dnsdist.servermem",
		Type:  collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "real-memory-usage", Name: "memory usage", Div: 1 << 20},
		},
	},
	{
		ID:    "query_latency",
		Title: "Query latency",
		Units: "queries/s",
		Fam:   "latency",
		Ctx:   "dnsdist.query_latency",
		Type:  collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "latency0-1", Name: "1ms", Algo: collectorapi.Incremental},
			{ID: "latency1-10", Name: "10ms", Algo: collectorapi.Incremental},
			{ID: "latency10-50", Name: "50ms", Algo: collectorapi.Incremental},
			{ID: "latency50-100", Name: "100ms", Algo: collectorapi.Incremental},
			{ID: "latency100-1000", Name: "1sec", Algo: collectorapi.Incremental},
			{ID: "latency-slow", Name: "slow", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "query_latency_avg",
		Title: "Average latency for the last N queries",
		Units: "microseconds",
		Fam:   "latency",
		Ctx:   "dnsdist.query_latency_avg",
		Dims: collectorapi.Dims{
			{ID: "latency-avg100", Name: "100"},
			{ID: "latency-avg1000", Name: "1k"},
			{ID: "latency-avg10000", Name: "10k"},
			{ID: "latency-avg1000000", Name: "1000k"},
		},
	},
}
