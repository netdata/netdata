// SPDX-License-Identifier: GPL-3.0-or-later

package tor

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioTraffic = module.Priority + iota
	prioUptime
)

var charts = module.Charts{
	trafficChart.Copy(),
	uptimeChart.Copy(),
}

var trafficChart = module.Chart{
	ID:       "traffic",
	Title:    "Tor Traffic",
	Units:    "KiB/s",
	Fam:      "traffic",
	Ctx:      "tor.traffic",
	Type:     module.Area,
	Priority: prioTraffic,
	Dims: module.Dims{
		{ID: "traffic/read", Name: "read", Algo: module.Incremental, Div: 1024},
		{ID: "traffic/written", Name: "write", Algo: module.Incremental, Mul: -1, Div: 1024},
	},
}

var uptimeChart = module.Chart{
	ID:       "uptime",
	Title:    "Tor Uptime",
	Units:    "seconds",
	Fam:      "uptime",
	Ctx:      "tor.uptime",
	Priority: prioUptime,
	Dims: module.Dims{
		{ID: "uptime"},
	},
}
