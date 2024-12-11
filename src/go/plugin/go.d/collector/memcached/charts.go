// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioCache = module.Priority + iota
	prioNet
	prioConnections
	prioItems
	prioEvictedReclaimed
	prioGet
	prioGetRate
	prioSetRate
	prioDelete
	prioCas
	prioIncrement
	prioDecrement
	prioTouch
	prioTouchRate
)

var charts = module.Charts{
	cacheChart.Copy(),
	netChart.Copy(),
	connectionsChart.Copy(),
	itemsChart.Copy(),
	EvictedReclaimedChart.Copy(),
	getChart.Copy(),
	getRateChart.Copy(),
	setRateChart.Copy(),
	deleteChart.Copy(),
	casChart.Copy(),
	incrementChart.Copy(),
	decrementChart.Copy(),
	touchChart.Copy(),
	touchRateChart.Copy(),
}

const (
	byteToMiB = 1 << 20
)

var (
	cacheChart = module.Chart{
		ID:       "cache",
		Title:    "Cache Size",
		Units:    "MiB",
		Fam:      "cache",
		Ctx:      "memcached.cache",
		Type:     module.Stacked,
		Priority: prioCache,
		Dims: module.Dims{
			{ID: "avail", Div: byteToMiB},
			{ID: "bytes", Name: "used", Div: byteToMiB},
		},
	}
	netChart = module.Chart{
		ID:       "net",
		Title:    "Network",
		Units:    "kilobits/s",
		Fam:      "network",
		Ctx:      "memcached.net",
		Type:     module.Area,
		Priority: prioNet,
		Dims: module.Dims{
			{ID: "bytes_read", Name: "in", Mul: 8, Div: 1000, Algo: module.Incremental},
			{ID: "bytes_written", Name: "out", Mul: -8, Div: 1000, Algo: module.Incremental},
		},
	}
	connectionsChart = module.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "memcached.connections",
		Type:     module.Line,
		Priority: prioConnections,
		Dims: module.Dims{
			{ID: "curr_connections", Name: "current", Algo: module.Incremental},
			{ID: "rejected_connections", Name: "rejected", Algo: module.Incremental},
			{ID: "total_connections", Name: "total", Algo: module.Incremental},
		},
	}
	itemsChart = module.Chart{
		ID:       "items",
		Title:    "Items",
		Units:    "items",
		Fam:      "items",
		Ctx:      "memcached.items",
		Type:     module.Line,
		Priority: prioItems,
		Dims: module.Dims{
			{ID: "curr_items", Name: "current"},
			{ID: "total_items", Name: "total"},
		},
	}
	EvictedReclaimedChart = module.Chart{
		ID:       "evicted_reclaimed",
		Title:    "Evicted and Reclaimed Items",
		Units:    "items",
		Fam:      "items",
		Ctx:      "memcached.evicted_reclaimed",
		Type:     module.Line,
		Priority: prioEvictedReclaimed,
		Dims: module.Dims{
			{ID: "reclaimed"},
			{ID: "evictions", Name: "evicted"},
		},
	}
	getChart = module.Chart{
		ID:       "get",
		Title:    "Get Requests",
		Units:    "requests",
		Fam:      "get ops",
		Ctx:      "memcached.get",
		Type:     module.Stacked,
		Priority: prioGet,
		Dims: module.Dims{
			{ID: "get_hits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "get_misses", Name: "misses", Algo: module.PercentOfAbsolute},
		},
	}
	getRateChart = module.Chart{
		ID:       "get_rate",
		Title:    "Get Request Rate",
		Units:    "requests/s",
		Fam:      "get ops",
		Ctx:      "memcached.get_rate",
		Type:     module.Line,
		Priority: prioGetRate,
		Dims: module.Dims{
			{ID: "cmd_get", Name: "rate", Algo: module.Incremental},
		},
	}
	setRateChart = module.Chart{
		ID:       "set_rate",
		Title:    "Set Request Rate",
		Units:    "requests/s",
		Fam:      "set ops",
		Ctx:      "memcached.set_rate",
		Type:     module.Line,
		Priority: prioSetRate,
		Dims: module.Dims{
			{ID: "cmd_set", Name: "rate", Algo: module.Incremental},
		},
	}
	deleteChart = module.Chart{
		ID:       "delete",
		Title:    "Delete Requests",
		Units:    "requests",
		Fam:      "delete ops",
		Ctx:      "memcached.delete",
		Type:     module.Stacked,
		Priority: prioDelete,
		Dims: module.Dims{
			{ID: "delete_hits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "delete_misses", Name: "misses", Algo: module.PercentOfAbsolute},
		},
	}
	casChart = module.Chart{
		ID:       "cas",
		Title:    "Check and Set Requests",
		Units:    "requests",
		Fam:      "check and set ops",
		Ctx:      "memcached.cas",
		Type:     module.Stacked,
		Priority: prioCas,
		Dims: module.Dims{
			{ID: "cas_hits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "cas_misses", Name: "misses", Algo: module.PercentOfAbsolute},
			{ID: "cas_badval", Name: "bad value", Algo: module.PercentOfAbsolute},
		},
	}
	incrementChart = module.Chart{
		ID:       "increment",
		Title:    "Increment Requests",
		Units:    "requests",
		Fam:      "increment ops",
		Ctx:      "memcached.increment",
		Type:     module.Stacked,
		Priority: prioIncrement,
		Dims: module.Dims{
			{ID: "incr_hits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "incr_misses", Name: "misses", Algo: module.PercentOfAbsolute},
		},
	}
	decrementChart = module.Chart{
		ID:       "decrement",
		Title:    "Decrement Requests",
		Units:    "requests",
		Fam:      "decrement ops",
		Ctx:      "memcached.decrement",
		Type:     module.Stacked,
		Priority: prioDecrement,
		Dims: module.Dims{
			{ID: "decr_hits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "decr_misses", Name: "misses", Algo: module.PercentOfAbsolute},
		},
	}
	touchChart = module.Chart{
		ID:       "touch",
		Title:    "Touch Requests",
		Units:    "requests",
		Fam:      "touch ops",
		Ctx:      "memcached.touch",
		Type:     module.Stacked,
		Priority: prioTouch,
		Dims: module.Dims{
			{ID: "touch_hits", Name: "hits", Algo: module.PercentOfAbsolute},
			{ID: "touch_misses", Name: "misses", Algo: module.PercentOfAbsolute},
		},
	}
	touchRateChart = module.Chart{
		ID:       "touch_rate",
		Title:    "Touch Requests Rate",
		Units:    "requests/s",
		Fam:      "touch ops",
		Ctx:      "memcached.touch_rate",
		Type:     module.Line,
		Priority: prioTouchRate,
		Dims: module.Dims{
			{ID: "cmd_touch", Name: "rate", Algo: module.Incremental},
		},
	}
)
