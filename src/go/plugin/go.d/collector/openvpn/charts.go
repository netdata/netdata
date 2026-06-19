// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "active_clients",
		Title: "Total Number Of Active Clients",
		Units: "clients",
		Fam:   "clients",
		Ctx:   "openvpn.active_clients",
		Dims: Dims{
			{ID: "clients"},
		},
	},
	{
		ID:    "total_traffic",
		Title: "Total Traffic",
		Units: "kilobits/s",
		Fam:   "traffic",
		Ctx:   "openvpn.total_traffic",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: "bytes_in", Name: "in", Algo: collectorapi.Incremental, Mul: 8, Div: 1000},
			{ID: "bytes_out", Name: "out", Algo: collectorapi.Incremental, Mul: 8, Div: -1000},
		},
	},
}

var userCharts = Charts{
	{
		ID:    "%s_user_traffic",
		Title: "User Traffic",
		Units: "kilobits/s",
		Fam:   "user %s",
		Ctx:   "openvpn.user_traffic",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: "%s_bytes_received", Name: "received", Algo: collectorapi.Incremental, Mul: 8, Div: 1000},
			{ID: "%s_bytes_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: 8, Div: -1000},
		},
	},
	{
		ID:    "%s_user_connection_time",
		Title: "User Connection Time",
		Units: "seconds",
		Fam:   "user %s",
		Ctx:   "openvpn.user_connection_time",
		Dims: Dims{
			{ID: "%s_connection_time", Name: "time"},
		},
	},
}
