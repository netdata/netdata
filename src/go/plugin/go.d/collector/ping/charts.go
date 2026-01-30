// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioHostRTT = module.Priority + iota
	prioHostStdDevRTT
	prioHostJitter
	prioHostVariance
	prioHostPingPacketLoss
	prioHostPingPackets
)

var (
	hostRTTChartTmpl = module.Chart{
		ID:       "host_%s_rtt",
		Title:    "Ping round-trip time",
		Units:    "milliseconds",
		Fam:      "latency",
		Ctx:      "ping.host_rtt",
		Priority: prioHostRTT,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "host_%s_min_rtt", Name: "min", Div: 1e3},
			{ID: "host_%s_max_rtt", Name: "max", Div: 1e3},
			{ID: "host_%s_avg_rtt", Name: "avg", Div: 1e3},
		},
	}
	hostStdDevRTTChartTmpl = module.Chart{
		ID:       "host_%s_std_dev_rtt",
		Title:    "Ping round-trip time standard deviation",
		Units:    "milliseconds",
		Fam:      "latency",
		Ctx:      "ping.host_std_dev_rtt",
		Priority: prioHostStdDevRTT,
		Dims: module.Dims{
			{ID: "host_%s_std_dev_rtt", Name: "std_dev", Div: 1e3},
		},
	}
	hostJitterChartTmpl = module.Chart{
		ID:       "host_%s_jitter",
		Title:    "Ping latency jitter",
		Units:    "milliseconds",
		Fam:      "latency",
		Ctx:      "ping.host_jitter",
		Priority: prioHostJitter,
		Dims: module.Dims{
			{ID: "host_%s_mean_jitter", Name: "mean", Div: 1e3},
			{ID: "host_%s_ewma_jitter", Name: "ewma", Div: 1e3},
			{ID: "host_%s_sma_jitter", Name: "sma", Div: 1e3},
		},
	}
	hostVarianceChartTmpl = module.Chart{
		ID:       "host_%s_rtt_variance",
		Title:    "Ping round-trip time variance",
		Units:    "ms²",
		Fam:      "latency",
		Ctx:      "ping.host_rtt_variance",
		Priority: prioHostVariance,
		Dims: module.Dims{
			{ID: "host_%s_rtt_variance", Name: "variance", Div: 1e6},
		},
	}
	hostPacketLossChartTmpl = module.Chart{
		ID:       "host_%s_packet_loss",
		Title:    "Ping packet loss",
		Units:    "percentage",
		Fam:      "packet loss",
		Ctx:      "ping.host_packet_loss",
		Priority: prioHostPingPacketLoss,
		Dims: module.Dims{
			{ID: "host_%s_packet_loss", Name: "loss", Div: 1000},
		},
	}
	hostPacketsChartTmpl = module.Chart{
		ID:       "host_%s_packets",
		Title:    "Ping packets transferred",
		Units:    "packets",
		Fam:      "packets",
		Ctx:      "ping.host_packets",
		Priority: prioHostPingPackets,
		Dims: module.Dims{
			{ID: "host_%s_packets_recv", Name: "received"},
			{ID: "host_%s_packets_sent", Name: "sent"},
		},
	}
)

var hostChartsTmpl = module.Charts{
	hostRTTChartTmpl.Copy(),
	hostStdDevRTTChartTmpl.Copy(),
	hostJitterChartTmpl.Copy(),
	hostVarianceChartTmpl.Copy(),
	hostPacketLossChartTmpl.Copy(),
	hostPacketsChartTmpl.Copy(),
}

func newHostCharts(host string) *module.Charts {
	charts := hostChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ReplaceAll(host, ".", "_"))
		chart.Labels = []module.Label{
			{Key: "host", Value: host},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, host)
		}
	}

	return charts
}

func (c *Collector) addHostCharts(host string) {
	charts := newHostCharts(host)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
