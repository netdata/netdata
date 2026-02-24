// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var charts = collectorapi.Charts{
	{
		ID:    "questions_in",
		Title: "Incoming questions",
		Units: "questions/s",
		Fam:   "questions",
		Ctx:   "powerdns_recursor.questions_in",
		Dims: collectorapi.Dims{
			{ID: "questions", Name: "total", Algo: collectorapi.Incremental},
			{ID: "tcp-questions", Name: "tcp", Algo: collectorapi.Incremental},
			{ID: "ipv6-questions", Name: "ipv6", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "questions_out",
		Title: "Outgoing questions",
		Units: "questions/s",
		Fam:   "questions",
		Ctx:   "powerdns_recursor.questions_out",
		Dims: collectorapi.Dims{
			{ID: "all-outqueries", Name: "udp", Algo: collectorapi.Incremental},
			{ID: "tcp-outqueries", Name: "tcp", Algo: collectorapi.Incremental},
			{ID: "ipv6-outqueries", Name: "ipv6", Algo: collectorapi.Incremental},
			{ID: "throttled-outqueries", Name: "throttled", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "answer_time",
		Title: "Queries answered within a time range",
		Units: "queries/s",
		Fam:   "performance",
		Ctx:   "powerdns_recursor.answer_time",
		Dims: collectorapi.Dims{
			{ID: "answers0-1", Name: "0-1ms", Algo: collectorapi.Incremental},
			{ID: "answers1-10", Name: "1-10ms", Algo: collectorapi.Incremental},
			{ID: "answers10-100", Name: "10-100ms", Algo: collectorapi.Incremental},
			{ID: "answers100-1000", Name: "100-1000ms", Algo: collectorapi.Incremental},
			{ID: "answers-slow", Name: "slow", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "timeouts",
		Title: "Timeouts on outgoing UDP queries",
		Units: "timeouts/s",
		Fam:   "performance",
		Ctx:   "powerdns_recursor.timeouts",
		Dims: collectorapi.Dims{
			{ID: "outgoing-timeouts", Name: "total", Algo: collectorapi.Incremental},
			{ID: "outgoing4-timeouts", Name: "ipv4", Algo: collectorapi.Incremental},
			{ID: "outgoing6-timeouts", Name: "ipv6", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "drops",
		Title: "Drops",
		Units: "drops/s",
		Fam:   "performance",
		Ctx:   "powerdns_recursor.drops",
		Dims: collectorapi.Dims{
			{ID: "over-capacity-drops", Algo: collectorapi.Incremental},
			{ID: "query-pipe-full-drops", Algo: collectorapi.Incremental},
			{ID: "too-old-drops", Algo: collectorapi.Incremental},
			{ID: "truncated-drops", Algo: collectorapi.Incremental},
			{ID: "empty-queries", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_usage",
		Title: "Cache Usage",
		Units: "events/s",
		Fam:   "cache",
		Ctx:   "powerdns_recursor.cache_usage",
		Dims: collectorapi.Dims{
			{ID: "cache-hits", Algo: collectorapi.Incremental},
			{ID: "cache-misses", Algo: collectorapi.Incremental},
			{ID: "packetcache-hits", Name: "packet-cache-hits", Algo: collectorapi.Incremental},
			{ID: "packetcache-misses", Name: "packet-cache-misses", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_size",
		Title: "Cache Size",
		Units: "entries",
		Fam:   "cache",
		Ctx:   "powerdns_recursor.cache_size",
		Dims: collectorapi.Dims{
			{ID: "cache-entries", Name: "cache"},
			{ID: "packetcache-entries", Name: "packet-cache"},
			{ID: "negcache-entries", Name: "negative-cache"},
		},
	},
}
