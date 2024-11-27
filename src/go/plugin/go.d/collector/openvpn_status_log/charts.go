// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var charts = module.Charts{
	{
		ID:    "active_clients",
		Title: "Active Clients",
		Units: "active clients",
		Fam:   "active_clients",
		Ctx:   "openvpn.active_clients",
		Dims: module.Dims{
			{ID: "clients"},
		},
	},
	{
		ID:    "traffic",
		Title: "Traffic",
		Units: "kilobits/s",
		Fam:   "traffic",
		Ctx:   "openvpn.total_traffic",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "bytes_in", Name: "in", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "bytes_out", Name: "out", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	},
}

var userCharts = module.Charts{
	{
		ID:    "%s_user_traffic",
		Title: "User Traffic",
		Units: "kilobits/s",
		Fam:   "user stats",
		Ctx:   "openvpn.user_traffic",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "%s_bytes_in", Name: "in", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "%s_bytes_out", Name: "out", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	},
	{
		ID:    "%s_user_connection_time",
		Title: "User Connection Time",
		Units: "seconds",
		Fam:   "user stats",
		Ctx:   "openvpn.user_connection_time",
		Dims: module.Dims{
			{ID: "%s_connection_time", Name: "time"},
		},
	},
}

func (c *Collector) addUserCharts(userName string) error {
	cs := userCharts.Copy()

	for _, chart := range *cs {
		chart.ID = fmt.Sprintf(chart.ID, userName)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, userName)
		}
		chart.MarkNotCreated()
	}
	return c.charts.Add(*cs...)
}
