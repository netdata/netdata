// SPDX-License-Identifier: GPL-3.0-or-later

package ap

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClients = module.Priority + iota
	prioBandwidth
	prioPackets
	prioIssues
	prioSignal
	prioBitrate
)

var apChartsTmpl = module.Charts{
	apClientsChart.Copy(),
	apBandwidthChart.Copy(),
	apPacketsChart.Copy(),
	apIssuesChart.Copy(),
	apSignalChart.Copy(),
	apBitrateChart.Copy(),
}

var (
	apClientsChart = module.Chart{
		ID:       "ap_clients.%s",
		Title:    "Connected clients to %s on %s",
		Units:    "clients",
		Fam:      "%s",
		Ctx:      "ap.clients",
		Type:     module.Line,
		Priority: prioClients,
		Dims: module.Dims{
			{ID: "ap_%s_clients", Name: "clients"},
		},
	}

	apBandwidthChart = module.Chart{
		ID:       "ap_bandwidth.%s",
		Title:    "Bandwidth for %s on %s",
		Units:    "kilobits/s",
		Fam:      "%s",
		Ctx:      "ap.net",
		Type:     module.Area,
		Priority: prioBandwidth,
		Dims: module.Dims{
			{ID: "ap_%s_bw_received", Name: "received", Algo: module.Incremental, Mul: 8, Div: 1024},
			{ID: "ap_%s_bw_sent", Name: "sent", Algo: module.Incremental, Mul: -8, Div: 1024},
		},
	}

	apPacketsChart = module.Chart{
		ID:       "ap_packets.%s",
		Title:    "Packets for %s on %s",
		Units:    "packets/s",
		Fam:      "%s",
		Ctx:      "ap.packets",
		Type:     module.Line,
		Priority: prioPackets,
		Dims: module.Dims{
			{ID: "ap_%s_packets_received", Name: "tx received", Algo: module.Incremental},
			{ID: "ap_%s_packets_sent", Name: "tx failures", Algo: module.Incremental, Mul: -1},
		},
	}

	apIssuesChart = module.Chart{
		ID:       "ap_issues.%s",
		Title:    "Transmit issues for %s on %s",
		Units:    "issues/s",
		Fam:      "%s",
		Ctx:      "ap.issues",
		Type:     module.Line,
		Priority: prioIssues,
		Dims: module.Dims{
			{ID: "ap_%s_issues_retries", Name: "tx retries", Algo: module.Incremental},
			{ID: "ap_%s_issues_failures", Name: "tx failures", Algo: module.Incremental, Mul: -1},
		},
	}

	apSignalChart = module.Chart{
		ID:       "ap_signal.%s",
		Title:    "Average Signal for %s on %s",
		Units:    "dBm",
		Fam:      "%s",
		Ctx:      "ap.signal",
		Type:     module.Line,
		Priority: prioSignal,
		Dims: module.Dims{
			{ID: "ap_%s_average_signal", Name: "average signal", Div: 1000},
		},
	}

	apBitrateChart = module.Chart{
		ID:       "ap_bitrate.%s",
		Title:    "Bitrate for %s on %s",
		Units:    "Mbps",
		Fam:      "%s",
		Ctx:      "ap.bitrate",
		Type:     module.Line,
		Priority: prioBitrate,
		Dims: module.Dims{
			{ID: "ap_%s_bitrate_receive", Name: "receive", Div: 1000},
			{ID: "ap_%s_bitrate_transmit", Name: "transmit", Mul: -1, Div: 1000},
			// deprecated dim, not present in command output
			// {ID: "ap_%s_bitrate_expected", Name: "expected throughput", Div: 1000},
		},
	}
)

func (a *AP) addInterfaceCharts(dev string, ssid string) {

	charts := apChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dev)
		chart.Title = fmt.Sprintf(chart.Title, ssid, dev)
		chart.Fam = fmt.Sprintf(chart.Fam, dev)
		chart.Labels = []module.Label{
			{Key: "ssid", Value: ssid},
			{Key: "device", Value: dev},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dev)
		}

	}

	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}

}

func (a *AP) removeInterfaceCharts(dev string) {
	for _, chart := range *a.Charts() {
		if strings.HasSuffix(chart.ID, dev) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
