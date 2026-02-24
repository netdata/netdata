// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioCache = collectorapi.Priority + iota
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

var charts = collectorapi.Charts{
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
	cacheChart = collectorapi.Chart{
		ID:       "cache",
		Title:    "Cache Size",
		Units:    "MiB",
		Fam:      "cache",
		Ctx:      "memcached.cache",
		Type:     collectorapi.Stacked,
		Priority: prioCache,
		Dims: collectorapi.Dims{
			{ID: "avail", Div: byteToMiB},
			{ID: "bytes", Name: "used", Div: byteToMiB},
		},
	}
	netChart = collectorapi.Chart{
		ID:       "net",
		Title:    "Network",
		Units:    "kilobits/s",
		Fam:      "network",
		Ctx:      "memcached.net",
		Type:     collectorapi.Area,
		Priority: prioNet,
		Dims: collectorapi.Dims{
			{ID: "bytes_read", Name: "in", Mul: 8, Div: 1000, Algo: collectorapi.Incremental},
			{ID: "bytes_written", Name: "out", Mul: -8, Div: 1000, Algo: collectorapi.Incremental},
		},
	}
	connectionsChart = collectorapi.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "memcached.connections",
		Type:     collectorapi.Line,
		Priority: prioConnections,
		Dims: collectorapi.Dims{
			{ID: "curr_connections", Name: "current", Algo: collectorapi.Incremental},
			{ID: "rejected_connections", Name: "rejected", Algo: collectorapi.Incremental},
			{ID: "total_connections", Name: "total", Algo: collectorapi.Incremental},
		},
	}
	itemsChart = collectorapi.Chart{
		ID:       "items",
		Title:    "Items",
		Units:    "items",
		Fam:      "items",
		Ctx:      "memcached.items",
		Type:     collectorapi.Line,
		Priority: prioItems,
		Dims: collectorapi.Dims{
			{ID: "curr_items", Name: "current"},
			{ID: "total_items", Name: "total"},
		},
	}
	EvictedReclaimedChart = collectorapi.Chart{
		ID:       "evicted_reclaimed",
		Title:    "Evicted and Reclaimed Items",
		Units:    "items",
		Fam:      "items",
		Ctx:      "memcached.evicted_reclaimed",
		Type:     collectorapi.Line,
		Priority: prioEvictedReclaimed,
		Dims: collectorapi.Dims{
			{ID: "reclaimed"},
			{ID: "evictions", Name: "evicted"},
		},
	}
	getChart = collectorapi.Chart{
		ID:       "get",
		Title:    "Get Requests",
		Units:    "requests",
		Fam:      "get ops",
		Ctx:      "memcached.get",
		Type:     collectorapi.Stacked,
		Priority: prioGet,
		Dims: collectorapi.Dims{
			{ID: "get_hits", Name: "hits", Algo: collectorapi.PercentOfAbsolute},
			{ID: "get_misses", Name: "misses", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	getRateChart = collectorapi.Chart{
		ID:       "get_rate",
		Title:    "Get Request Rate",
		Units:    "requests/s",
		Fam:      "get ops",
		Ctx:      "memcached.get_rate",
		Type:     collectorapi.Line,
		Priority: prioGetRate,
		Dims: collectorapi.Dims{
			{ID: "cmd_get", Name: "rate", Algo: collectorapi.Incremental},
		},
	}
	setRateChart = collectorapi.Chart{
		ID:       "set_rate",
		Title:    "Set Request Rate",
		Units:    "requests/s",
		Fam:      "set ops",
		Ctx:      "memcached.set_rate",
		Type:     collectorapi.Line,
		Priority: prioSetRate,
		Dims: collectorapi.Dims{
			{ID: "cmd_set", Name: "rate", Algo: collectorapi.Incremental},
		},
	}
	deleteChart = collectorapi.Chart{
		ID:       "delete",
		Title:    "Delete Requests",
		Units:    "requests",
		Fam:      "delete ops",
		Ctx:      "memcached.delete",
		Type:     collectorapi.Stacked,
		Priority: prioDelete,
		Dims: collectorapi.Dims{
			{ID: "delete_hits", Name: "hits", Algo: collectorapi.PercentOfAbsolute},
			{ID: "delete_misses", Name: "misses", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	casChart = collectorapi.Chart{
		ID:       "cas",
		Title:    "Check and Set Requests",
		Units:    "requests",
		Fam:      "check and set ops",
		Ctx:      "memcached.cas",
		Type:     collectorapi.Stacked,
		Priority: prioCas,
		Dims: collectorapi.Dims{
			{ID: "cas_hits", Name: "hits", Algo: collectorapi.PercentOfAbsolute},
			{ID: "cas_misses", Name: "misses", Algo: collectorapi.PercentOfAbsolute},
			{ID: "cas_badval", Name: "bad value", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	incrementChart = collectorapi.Chart{
		ID:       "increment",
		Title:    "Increment Requests",
		Units:    "requests",
		Fam:      "increment ops",
		Ctx:      "memcached.increment",
		Type:     collectorapi.Stacked,
		Priority: prioIncrement,
		Dims: collectorapi.Dims{
			{ID: "incr_hits", Name: "hits", Algo: collectorapi.PercentOfAbsolute},
			{ID: "incr_misses", Name: "misses", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	decrementChart = collectorapi.Chart{
		ID:       "decrement",
		Title:    "Decrement Requests",
		Units:    "requests",
		Fam:      "decrement ops",
		Ctx:      "memcached.decrement",
		Type:     collectorapi.Stacked,
		Priority: prioDecrement,
		Dims: collectorapi.Dims{
			{ID: "decr_hits", Name: "hits", Algo: collectorapi.PercentOfAbsolute},
			{ID: "decr_misses", Name: "misses", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	touchChart = collectorapi.Chart{
		ID:       "touch",
		Title:    "Touch Requests",
		Units:    "requests",
		Fam:      "touch ops",
		Ctx:      "memcached.touch",
		Type:     collectorapi.Stacked,
		Priority: prioTouch,
		Dims: collectorapi.Dims{
			{ID: "touch_hits", Name: "hits", Algo: collectorapi.PercentOfAbsolute},
			{ID: "touch_misses", Name: "misses", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	touchRateChart = collectorapi.Chart{
		ID:       "touch_rate",
		Title:    "Touch Requests Rate",
		Units:    "requests/s",
		Fam:      "touch ops",
		Ctx:      "memcached.touch_rate",
		Type:     collectorapi.Line,
		Priority: prioTouchRate,
		Dims: collectorapi.Dims{
			{ID: "cmd_touch", Name: "rate", Algo: collectorapi.Incremental},
		},
	}
)
