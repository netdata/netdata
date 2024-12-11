// SPDX-License-Identifier: GPL-3.0-or-later

package nginxunit

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioRequestsRate = module.Priority + iota
	prioConnectionsRate
	prioConnectionsCurrent
)

var charts = module.Charts{
	requestsRateChart.Copy(),
	connectionsRateChart.Copy(),
	connectionsCurrentChart.Copy(),
}

var requestsRateChart = module.Chart{
	ID:       "requests",
	Title:    "Requests",
	Units:    "requests/s",
	Fam:      "requests",
	Ctx:      "nginxunit.requests_rate",
	Priority: prioRequestsRate,
	Dims: module.Dims{
		{ID: "requests_total", Name: "requests", Algo: module.Incremental},
	},
}

var connectionsRateChart = module.Chart{
	ID:       "connections_rate",
	Title:    "Connections",
	Units:    "connections/s",
	Fam:      "connections",
	Ctx:      "nginxunit.connections_rate",
	Priority: prioConnectionsRate,
	Type:     module.Stacked,
	Dims: module.Dims{
		{ID: "connections_accepted", Name: "accepted", Algo: module.Incremental},
		{ID: "connections_closed", Name: "closed", Algo: module.Incremental},
	},
}

var connectionsCurrentChart = module.Chart{
	ID:       "connections_current",
	Title:    "Current Connections",
	Units:    "connections",
	Fam:      "connections",
	Ctx:      "nginxunit.connections_current",
	Priority: prioConnectionsCurrent,
	Type:     module.Stacked,
	Dims: module.Dims{
		{ID: "connections_active", Name: "active"},
		{ID: "connections_idle", Name: "idle"},
	},
}
