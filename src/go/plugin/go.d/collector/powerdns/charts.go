// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var charts = module.Charts{
	{
		ID:    "questions_in",
		Title: "Incoming questions",
		Units: "questions/s",
		Fam:   "questions",
		Ctx:   "powerdns.questions_in",
		Dims: module.Dims{
			{ID: "udp-queries", Name: "udp", Algo: module.Incremental},
			{ID: "tcp-queries", Name: "tcp", Algo: module.Incremental},
		},
	},
	{
		ID:    "questions_out",
		Title: "Outgoing questions",
		Units: "questions/s",
		Fam:   "questions",
		Ctx:   "powerdns.questions_out",
		Dims: module.Dims{
			{ID: "udp-answers", Name: "udp", Algo: module.Incremental},
			{ID: "tcp-answers", Name: "tcp", Algo: module.Incremental},
		},
	},
	{
		ID:    "cache_usage",
		Title: "Cache Usage",
		Units: "events/s",
		Fam:   "cache",
		Ctx:   "powerdns.cache_usage",
		Dims: module.Dims{
			{ID: "query-cache-hit", Algo: module.Incremental},
			{ID: "query-cache-miss", Algo: module.Incremental},
			{ID: "packetcache-hit", Name: "packet-cache-hit", Algo: module.Incremental},
			{ID: "packetcache-miss", Name: "packet-cache-miss", Algo: module.Incremental},
		},
	},
	{
		ID:    "cache_size",
		Title: "Cache Size",
		Units: "entries",
		Fam:   "cache",
		Ctx:   "powerdns.cache_size",
		Dims: module.Dims{
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
		Dims: module.Dims{
			{ID: "latency"},
		},
	},
}
