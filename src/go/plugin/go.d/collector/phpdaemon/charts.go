// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Chart is an alias for collectorapi.Chart
	Chart = collectorapi.Chart
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "workers",
		Title: "Workers",
		Units: "workers",
		Fam:   "workers",
		Ctx:   "phpdaemon.workers",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: "alive"},
			{ID: "shutdown"},
		},
	},
	{
		ID:    "alive_workers",
		Title: "Alive Workers State",
		Units: "workers",
		Fam:   "workers",
		Ctx:   "phpdaemon.alive_workers",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: "idle"},
			{ID: "busy"},
			{ID: "reloading"},
		},
	},
	{
		ID:    "idle_workers",
		Title: "Idle Workers State",
		Units: "workers",
		Fam:   "workers",
		Ctx:   "phpdaemon.idle_workers",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: "preinit"},
			{ID: "init"},
			{ID: "initialized"},
		},
	},
}

var uptimeChart = Chart{
	ID:    "uptime",
	Title: "Uptime",
	Units: "seconds",
	Fam:   "uptime",
	Ctx:   "phpdaemon.uptime",
	Dims: Dims{
		{ID: "uptime", Name: "time"},
	},
}
