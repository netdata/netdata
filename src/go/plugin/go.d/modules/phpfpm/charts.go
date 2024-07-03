// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dims is an alias for module.Dims
	Dims = module.Dims
)

var charts = Charts{
	{
		ID:    "connections",
		Title: "Active Connections",
		Units: "connections",
		Fam:   "active connections",
		Ctx:   "phpfpm.connections",
		Dims: Dims{
			{ID: "active"},
			{ID: "maxActive", Name: "max active"},
			{ID: "idle"},
		},
	},
	{
		ID:    "requests",
		Title: "Requests",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "phpfpm.requests",
		Dims: Dims{
			{ID: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:    "performance",
		Title: "Performance",
		Units: "status",
		Fam:   "performance",
		Ctx:   "phpfpm.performance",
		Dims: Dims{
			{ID: "reached", Name: "max children reached"},
			{ID: "slow", Name: "slow requests"},
		},
	},
	{
		ID:    "request_duration",
		Title: "Requests Duration Among All Idle Processes",
		Units: "milliseconds",
		Fam:   "request duration",
		Ctx:   "phpfpm.request_duration",
		Dims: Dims{
			{ID: "minReqDur", Name: "min", Div: 1000},
			{ID: "maxReqDur", Name: "max", Div: 1000},
			{ID: "avgReqDur", Name: "avg", Div: 1000},
		},
	},
	{
		ID:    "request_cpu",
		Title: "Last Request CPU Usage Among All Idle Processes",
		Units: "percentage",
		Fam:   "request CPU",
		Ctx:   "phpfpm.request_cpu",
		Dims: Dims{
			{ID: "minReqCpu", Name: "min"},
			{ID: "maxReqCpu", Name: "max"},
			{ID: "avgReqCpu", Name: "avg"},
		},
	},
	{
		ID:    "request_mem",
		Title: "Last Request Memory Usage Among All Idle Processes",
		Units: "KB",
		Fam:   "request memory",
		Ctx:   "phpfpm.request_mem",
		Dims: Dims{
			{ID: "minReqMem", Name: "min", Div: 1024},
			{ID: "maxReqMem", Name: "max", Div: 1024},
			{ID: "avgReqMem", Name: "avg", Div: 1024},
		},
	},
}
