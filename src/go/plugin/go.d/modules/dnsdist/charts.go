// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var charts = module.Charts{
	{
		ID:    "queries",
		Title: "Client queries received",
		Units: "queries/s",
		Fam:   "queries",
		Ctx:   "dnsdist.queries",
		Dims: module.Dims{
			{ID: "queries", Name: "all", Algo: module.Incremental},
			{ID: "rdqueries", Name: "recursive", Algo: module.Incremental},
			{ID: "empty-queries", Name: "empty", Algo: module.Incremental},
		},
	},
	{
		ID:    "queries_dropped",
		Title: "Client queries dropped",
		Units: "queries/s",
		Fam:   "queries",
		Ctx:   "dnsdist.queries_dropped",
		Dims: module.Dims{
			{ID: "rule-drop", Name: "rule drop", Algo: module.Incremental},
			{ID: "dyn-blocked", Name: "dynamic blocked", Algo: module.Incremental},
			{ID: "no-policy", Name: "no policy", Algo: module.Incremental},
			{ID: "noncompliant-queries", Name: "non queries", Algo: module.Incremental},
		},
	},
	{
		ID:    "packets_dropped",
		Title: "Packets dropped",
		Units: "packets/s",
		Fam:   "packets",
		Ctx:   "dnsdist.packets_dropped",
		Dims: module.Dims{
			{ID: "acl-drops", Name: "acl", Algo: module.Incremental},
		},
	},
	{
		ID:    "answers",
		Title: "Answers statistics",
		Units: "answers/s",
		Fam:   "answers",
		Ctx:   "dnsdist.answers",
		Dims: module.Dims{
			{ID: "self-answered", Name: "self answered", Algo: module.Incremental},
			{ID: "rule-nxdomain", Name: "nxdomain", Algo: module.Incremental, Mul: -1},
			{ID: "rule-refused", Name: "refused", Algo: module.Incremental, Mul: -1},
			{ID: "trunc-failures", Name: "trunc failures", Algo: module.Incremental, Mul: -1},
		},
	},
	{
		ID:    "backend_responses",
		Title: "Backend responses",
		Units: "responses/s",
		Fam:   "backends",
		Ctx:   "dnsdist.backend_responses",
		Dims: module.Dims{
			{ID: "responses", Name: "responses", Algo: module.Incremental},
		},
	},
	{
		ID:    "backend_commerrors",
		Title: "Backend communication errors",
		Units: "errors/s",
		Fam:   "backends",
		Ctx:   "dnsdist.backend_commerrors",
		Dims: module.Dims{
			{ID: "downstream-send-errors", Name: "send errors", Algo: module.Incremental},
		},
	},
	{
		ID:    "backend_errors",
		Title: "Backend error responses",
		Units: "responses/s",
		Fam:   "backends",
		Ctx:   "dnsdist.backend_errors",
		Dims: module.Dims{
			{ID: "downstream-timeouts", Name: "timeouts", Algo: module.Incremental},
			{ID: "servfail-responses", Name: "servfail", Algo: module.Incremental},
			{ID: "noncompliant-responses", Name: "non compliant", Algo: module.Incremental},
		},
	},
	{
		ID:    "cache",
		Title: "Cache performance",
		Units: "answers/s",
		Fam:   "cache",
		Ctx:   "dnsdist.cache",
		Dims: module.Dims{
			{ID: "cache-hits", Name: "hits", Algo: module.Incremental},
			{ID: "cache-misses", Name: "misses", Algo: module.Incremental, Mul: -1},
		},
	},
	{
		ID:    "servercpu",
		Title: "DNSdist server CPU utilization",
		Units: "ms/s",
		Fam:   "server",
		Ctx:   "dnsdist.servercpu",
		Type:  module.Stacked,
		Dims: module.Dims{
			{ID: "cpu-sys-msec", Name: "system state", Algo: module.Incremental},
			{ID: "cpu-user-msec", Name: "user state", Algo: module.Incremental},
		},
	},
	{
		ID:    "servermem",
		Title: "DNSdist server memory utilization",
		Units: "MiB",
		Fam:   "server",
		Ctx:   "dnsdist.servermem",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "real-memory-usage", Name: "memory usage", Div: 1 << 20},
		},
	},
	{
		ID:    "query_latency",
		Title: "Query latency",
		Units: "queries/s",
		Fam:   "latency",
		Ctx:   "dnsdist.query_latency",
		Type:  module.Stacked,
		Dims: module.Dims{
			{ID: "latency0-1", Name: "1ms", Algo: module.Incremental},
			{ID: "latency1-10", Name: "10ms", Algo: module.Incremental},
			{ID: "latency10-50", Name: "50ms", Algo: module.Incremental},
			{ID: "latency50-100", Name: "100ms", Algo: module.Incremental},
			{ID: "latency100-1000", Name: "1sec", Algo: module.Incremental},
			{ID: "latency-slow", Name: "slow", Algo: module.Incremental},
		},
	},
	{
		ID:    "query_latency_avg",
		Title: "Average latency for the last N queries",
		Units: "microseconds",
		Fam:   "latency",
		Ctx:   "dnsdist.query_latency_avg",
		Dims: module.Dims{
			{ID: "latency-avg100", Name: "100"},
			{ID: "latency-avg1000", Name: "1k"},
			{ID: "latency-avg10000", Name: "10k"},
			{ID: "latency-avg1000000", Name: "1000k"},
		},
	},
}
