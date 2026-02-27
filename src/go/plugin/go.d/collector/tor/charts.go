// SPDX-License-Identifier: GPL-3.0-or-later

package tor

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioTraffic = collectorapi.Priority + iota
	prioUptime
)

var charts = collectorapi.Charts{
	trafficChart.Copy(),
	uptimeChart.Copy(),
}

var trafficChart = collectorapi.Chart{
	ID:       "traffic",
	Title:    "Tor Traffic",
	Units:    "KiB/s",
	Fam:      "traffic",
	Ctx:      "tor.traffic",
	Type:     collectorapi.Area,
	Priority: prioTraffic,
	Dims: collectorapi.Dims{
		{ID: "traffic/read", Name: "read", Algo: collectorapi.Incremental, Div: 1024},
		{ID: "traffic/written", Name: "write", Algo: collectorapi.Incremental, Mul: -1, Div: 1024},
	},
}

var uptimeChart = collectorapi.Chart{
	ID:       "uptime",
	Title:    "Tor Uptime",
	Units:    "seconds",
	Fam:      "uptime",
	Ctx:      "tor.uptime",
	Priority: prioUptime,
	Dims: collectorapi.Dims{
		{ID: "uptime"},
	},
}
