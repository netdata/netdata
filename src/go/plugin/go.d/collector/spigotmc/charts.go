// SPDX-License-Identifier: GPL-3.0-or-later

package spigotmc

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioPlayers = module.Priority + iota
	prioTps
	prioMemory
)

var charts = module.Charts{
	playersChart.Copy(),
	tpsChart.Copy(),
	memoryChart.Copy(),
}

var playersChart = module.Chart{
	ID:       "players",
	Title:    "Active Players",
	Units:    "players",
	Fam:      "players",
	Ctx:      "spigotmc.players",
	Priority: prioPlayers,
	Dims: module.Dims{
		{ID: "players", Name: "players"},
	},
}

var tpsChart = module.Chart{
	ID:       "avg_tps",
	Title:    "Average Ticks Per Second",
	Units:    "ticks",
	Fam:      "ticks",
	Ctx:      "spigotmc.avg_tps",
	Priority: prioTps,
	Dims: module.Dims{
		{ID: "tps_1min", Name: "1min", Div: precision},
		{ID: "tps_5min", Name: "5min", Div: precision},
		{ID: "tps_15min", Name: "15min", Div: precision},
	},
}

var memoryChart = module.Chart{
	ID:       "memory",
	Title:    "Memory Usage",
	Units:    "bytes",
	Fam:      "mem",
	Ctx:      "spigotmc.memory",
	Priority: prioMemory,
	Type:     module.Area,
	Dims: module.Dims{
		{ID: "mem_used", Name: "used"},
		{ID: "mem_alloc", Name: "alloc"},
	},
}
