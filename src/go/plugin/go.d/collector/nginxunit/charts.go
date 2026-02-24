// SPDX-License-Identifier: GPL-3.0-or-later

package nginxunit

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioRequestsRate = collectorapi.Priority + iota
	prioConnectionsRate
	prioConnectionsCurrent
)

var charts = collectorapi.Charts{
	requestsRateChart.Copy(),
	connectionsRateChart.Copy(),
	connectionsCurrentChart.Copy(),
}

var requestsRateChart = collectorapi.Chart{
	ID:       "requests",
	Title:    "Requests",
	Units:    "requests/s",
	Fam:      "requests",
	Ctx:      "nginxunit.requests_rate",
	Priority: prioRequestsRate,
	Dims: collectorapi.Dims{
		{ID: "requests_total", Name: "requests", Algo: collectorapi.Incremental},
	},
}

var connectionsRateChart = collectorapi.Chart{
	ID:       "connections_rate",
	Title:    "Connections",
	Units:    "connections/s",
	Fam:      "connections",
	Ctx:      "nginxunit.connections_rate",
	Priority: prioConnectionsRate,
	Type:     collectorapi.Stacked,
	Dims: collectorapi.Dims{
		{ID: "connections_accepted", Name: "accepted", Algo: collectorapi.Incremental},
		{ID: "connections_closed", Name: "closed", Algo: collectorapi.Incremental},
	},
}

var connectionsCurrentChart = collectorapi.Chart{
	ID:       "connections_current",
	Title:    "Current Connections",
	Units:    "connections",
	Fam:      "connections",
	Ctx:      "nginxunit.connections_current",
	Priority: prioConnectionsCurrent,
	Type:     collectorapi.Stacked,
	Dims: collectorapi.Dims{
		{ID: "connections_active", Name: "active"},
		{ID: "connections_idle", Name: "idle"},
	},
}
