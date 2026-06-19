// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var charts = collectorapi.Charts{
	{
		ID:    "questions_in",
		Title: "Incoming questions",
		Units: "questions/s",
		Fam:   "questions",
		Ctx:   "powerdns.questions_in",
		Dims: collectorapi.Dims{
			{ID: "udp-queries", Name: "udp", Algo: collectorapi.Incremental},
			{ID: "tcp-queries", Name: "tcp", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "questions_out",
		Title: "Outgoing questions",
		Units: "questions/s",
		Fam:   "questions",
		Ctx:   "powerdns.questions_out",
		Dims: collectorapi.Dims{
			{ID: "udp-answers", Name: "udp", Algo: collectorapi.Incremental},
			{ID: "tcp-answers", Name: "tcp", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_usage",
		Title: "Cache Usage",
		Units: "events/s",
		Fam:   "cache",
		Ctx:   "powerdns.cache_usage",
		Dims: collectorapi.Dims{
			{ID: "query-cache-hit", Algo: collectorapi.Incremental},
			{ID: "query-cache-miss", Algo: collectorapi.Incremental},
			{ID: "packetcache-hit", Name: "packet-cache-hit", Algo: collectorapi.Incremental},
			{ID: "packetcache-miss", Name: "packet-cache-miss", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "cache_size",
		Title: "Cache Size",
		Units: "entries",
		Fam:   "cache",
		Ctx:   "powerdns.cache_size",
		Dims: collectorapi.Dims{
			{ID: "query-cache-size", Name: "query-cache"},
			{ID: "packetcache-size", Name: "packet-cache"},
			{ID: "key-cache-size", Name: "key-cache"},
			{ID: "meta-cache-size", Name: "meta-cache"},
		},
	},
	{
		ID:    "latency",
		Title: "Answer latency",
		Units: "microseconds",
		Fam:   "latency",
		Ctx:   "powerdns.latency",
		Dims: collectorapi.Dims{
			{ID: "latency"},
		},
	},
}
