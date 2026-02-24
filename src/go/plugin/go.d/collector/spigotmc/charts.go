// SPDX-License-Identifier: GPL-3.0-or-later

package spigotmc

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioPlayers = collectorapi.Priority + iota
	prioTps
	prioMemory
)

var charts = collectorapi.Charts{
	playersChart.Copy(),
	tpsChart.Copy(),
	memoryChart.Copy(),
}

var playersChart = collectorapi.Chart{
	ID:       "players",
	Title:    "Active Players",
	Units:    "players",
	Fam:      "players",
	Ctx:      "spigotmc.players",
	Priority: prioPlayers,
	Dims: collectorapi.Dims{
		{ID: "players", Name: "players"},
	},
}

var tpsChart = collectorapi.Chart{
	ID:       "avg_tps",
	Title:    "Average Ticks Per Second",
	Units:    "ticks",
	Fam:      "ticks",
	Ctx:      "spigotmc.avg_tps",
	Priority: prioTps,
	Dims: collectorapi.Dims{
		{ID: "tps_1min", Name: "1min", Div: precision},
		{ID: "tps_5min", Name: "5min", Div: precision},
		{ID: "tps_15min", Name: "15min", Div: precision},
	},
}

var memoryChart = collectorapi.Chart{
	ID:       "memory",
	Title:    "Memory Usage",
	Units:    "bytes",
	Fam:      "mem",
	Ctx:      "spigotmc.memory",
	Priority: prioMemory,
	Type:     collectorapi.Area,
	Dims: collectorapi.Dims{
		{ID: "mem_used", Name: "used"},
		{ID: "mem_alloc", Name: "alloc"},
	},
}
