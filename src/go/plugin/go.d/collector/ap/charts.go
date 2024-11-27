// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

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
	apClientsChartTmpl.Copy(),
	apBandwidthChartTmpl.Copy(),
	apPacketsChartTmpl.Copy(),
	apIssuesChartTmpl.Copy(),
	apSignalChartTmpl.Copy(),
	apBitrateChartTmpl.Copy(),
}

var (
	apClientsChartTmpl = module.Chart{
		ID:       "ap_%s_%s_clients",
		Title:    "Connected clients",
		Fam:      "clients",
		Units:    "clients",
		Ctx:      "ap.clients",
		Type:     module.Line,
		Priority: prioClients,
		Dims: module.Dims{
			{ID: "ap_%s_%s_clients", Name: "clients"},
		},
	}

	apBandwidthChartTmpl = module.Chart{
		ID:       "ap_%s_%s_bandwidth",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "traffic",
		Ctx:      "ap.net",
		Type:     module.Area,
		Priority: prioBandwidth,
		Dims: module.Dims{
			{ID: "ap_%s_%s_bw_received", Name: "received", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "ap_%s_%s_bw_sent", Name: "sent", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	}

	apPacketsChartTmpl = module.Chart{
		ID:       "ap_%s_%s_packets",
		Title:    "Packets",
		Fam:      "packets",
		Units:    "packets/s",
		Ctx:      "ap.packets",
		Type:     module.Line,
		Priority: prioPackets,
		Dims: module.Dims{
			{ID: "ap_%s_%s_packets_received", Name: "received", Algo: module.Incremental},
			{ID: "ap_%s_%s_packets_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}

	apIssuesChartTmpl = module.Chart{
		ID:       "ap_%s_%s_issues",
		Title:    "Transmit issues",
		Fam:      "issues",
		Units:    "issues/s",
		Ctx:      "ap.issues",
		Type:     module.Line,
		Priority: prioIssues,
		Dims: module.Dims{
			{ID: "ap_%s_%s_issues_retries", Name: "tx retries", Algo: module.Incremental},
			{ID: "ap_%s_%s_issues_failures", Name: "tx failures", Algo: module.Incremental, Mul: -1},
		},
	}

	apSignalChartTmpl = module.Chart{
		ID:       "ap_%s_%s_signal",
		Title:    "Average Signal",
		Units:    "dBm",
		Fam:      "signal",
		Ctx:      "ap.signal",
		Type:     module.Line,
		Priority: prioSignal,
		Dims: module.Dims{
			{ID: "ap_%s_%s_average_signal", Name: "average signal", Div: precision},
		},
	}

	apBitrateChartTmpl = module.Chart{
		ID:       "ap_%s_%s_bitrate",
		Title:    "Bitrate",
		Units:    "Mbps",
		Fam:      "bitrate",
		Ctx:      "ap.bitrate",
		Type:     module.Line,
		Priority: prioBitrate,
		Dims: module.Dims{
			{ID: "ap_%s_%s_bitrate_receive", Name: "receive", Div: precision},
			{ID: "ap_%s_%s_bitrate_transmit", Name: "transmit", Mul: -1, Div: precision},
		},
	}
)

func (c *Collector) addInterfaceCharts(dev *iwInterface) {
	charts := apChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dev.name, cleanSSID(dev.ssid))
		chart.Labels = []module.Label{
			{Key: "device", Value: dev.name},
			{Key: "ssid", Value: dev.ssid},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dev.name, dev.ssid)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}

}

func (c *Collector) removeInterfaceCharts(dev *iwInterface) {
	px := fmt.Sprintf("ap_%s_%s_", dev.name, cleanSSID(dev.ssid))
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanSSID(ssid string) string {
	r := strings.NewReplacer(" ", "_", ".", "_")
	return r.Replace(ssid)
}
