// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

const (
	prioTraffic = module.Priority + iota
	prioPackets
	prioFlows
	prioDropped
)

var (
	trafficChart = module.Chart{
		ID:       "traffic",
		Title:    "Flow traffic",
		Units:    "bytes/s",
		Fam:      "Traffic",
		Ctx:      "netflow.traffic",
		Priority: prioTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "netflow_bytes", Name: "bytes"},
		},
	}
	packetsChart = module.Chart{
		ID:       "packets",
		Title:    "Flow packets",
		Units:    "packets/s",
		Fam:      "Traffic",
		Ctx:      "netflow.packets",
		Priority: prioPackets,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "netflow_packets", Name: "packets"},
		},
	}
	flowsChart = module.Chart{
		ID:       "flows",
		Title:    "Flow records",
		Units:    "flows/s",
		Fam:      "Traffic",
		Ctx:      "netflow.flows",
		Priority: prioFlows,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "netflow_flows", Name: "flows"},
		},
	}
	droppedChart = module.Chart{
		ID:       "dropped",
		Title:    "Dropped flow records",
		Units:    "records/s",
		Fam:      "Quality",
		Ctx:      "netflow.dropped",
		Priority: prioDropped,
		Dims: module.Dims{
			{ID: "netflow_dropped", Name: "dropped"},
		},
	}
)

func (c *Collector) addCharts() error {
	return c.charts.Add(
		trafficChart.Copy(),
		packetsChart.Copy(),
		flowsChart.Copy(),
		droppedChart.Copy(),
	)
}
