// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioSystemOffset = collectorapi.Priority + iota
	prioSystemJitter
	prioSystemFrequency
	prioSystemWander
	prioSystemRootDelay
	prioSystemRootDispersion
	prioSystemStratum
	prioSystemTimeConstant
	prioSystemPrecision

	prioPeerOffset
	prioPeerDelay
	prioPeerDispersion
	prioPeerJitter
	prioPeerXleave
	prioPeerRootDelay
	prioPeerRootDispersion
	prioPeerStratum
	prioPeerHostMode
	prioPeerPeerMode
	prioPeerHostPoll
	prioPeerPeerPoll
	prioPeerPrecision
)

var (
	systemCharts = collectorapi.Charts{
		systemOffsetChart.Copy(),
		systemJitterChart.Copy(),
		systemFrequencyChart.Copy(),
		systemWanderChart.Copy(),
		systemRootDelayChart.Copy(),
		systemRootDispersionChart.Copy(),
		systemStratumChart.Copy(),
		systemTimeConstantChart.Copy(),
		systemPrecisionChart.Copy(),
	}
	systemOffsetChart = collectorapi.Chart{
		ID:       "sys_offset",
		Title:    "Combined offset of server relative to this host",
		Units:    "milliseconds",
		Fam:      "system",
		Ctx:      "ntpd.sys_offset",
		Type:     collectorapi.Area,
		Priority: prioSystemOffset,
		Dims: collectorapi.Dims{
			{ID: "offset", Name: "offset", Div: precision},
		},
	}
	systemJitterChart = collectorapi.Chart{
		ID:       "sys_jitter",
		Title:    "Combined system jitter and clock jitter",
		Units:    "milliseconds",
		Fam:      "system",
		Ctx:      "ntpd.sys_jitter",
		Priority: prioSystemJitter,
		Dims: collectorapi.Dims{
			{ID: "sys_jitter", Name: "system", Div: precision},
			{ID: "clk_jitter", Name: "clock", Div: precision},
		},
	}
	systemFrequencyChart = collectorapi.Chart{
		ID:       "sys_frequency",
		Title:    "Frequency offset relative to hardware clock",
		Units:    "ppm",
		Fam:      "system",
		Ctx:      "ntpd.sys_frequency",
		Type:     collectorapi.Area,
		Priority: prioSystemFrequency,
		Dims: collectorapi.Dims{
			{ID: "frequency", Name: "frequency", Div: precision},
		},
	}
	systemWanderChart = collectorapi.Chart{
		ID:       "sys_wander",
		Title:    "Clock frequency wander",
		Units:    "ppm",
		Fam:      "system",
		Ctx:      "ntpd.sys_wander",
		Type:     collectorapi.Area,
		Priority: prioSystemWander,
		Dims: collectorapi.Dims{
			{ID: "clk_wander", Name: "clock", Div: precision},
		},
	}
	systemRootDelayChart = collectorapi.Chart{
		ID:       "sys_rootdelay",
		Title:    "Total roundtrip delay to the primary reference clock",
		Units:    "milliseconds",
		Fam:      "system",
		Ctx:      "ntpd.sys_rootdelay",
		Type:     collectorapi.Area,
		Priority: prioSystemRootDelay,
		Dims: collectorapi.Dims{
			{ID: "rootdelay", Name: "delay", Div: precision},
		},
	}
	systemRootDispersionChart = collectorapi.Chart{
		ID:       "sys_rootdisp",
		Title:    "Total root dispersion to the primary reference clock",
		Units:    "milliseconds",
		Fam:      "system",
		Ctx:      "ntpd.sys_rootdisp",
		Type:     collectorapi.Area,
		Priority: prioSystemRootDispersion,
		Dims: collectorapi.Dims{
			{ID: "rootdisp", Name: "dispersion", Div: precision},
		},
	}
	systemStratumChart = collectorapi.Chart{
		ID:       "sys_stratum",
		Title:    "Stratum",
		Units:    "stratum",
		Fam:      "system",
		Ctx:      "ntpd.sys_stratum",
		Priority: prioSystemStratum,
		Dims: collectorapi.Dims{
			{ID: "stratum", Name: "stratum", Div: precision},
		},
	}
	systemTimeConstantChart = collectorapi.Chart{
		ID:       "sys_tc",
		Title:    "Time constant and poll exponent",
		Units:    "log2",
		Fam:      "system",
		Ctx:      "ntpd.sys_tc",
		Priority: prioSystemTimeConstant,
		Dims: collectorapi.Dims{
			{ID: "tc", Name: "current", Div: precision},
			{ID: "mintc", Name: "minimum", Div: precision},
		},
	}
	systemPrecisionChart = collectorapi.Chart{
		ID:       "sys_precision",
		Title:    "Precision",
		Units:    "log2",
		Fam:      "system",
		Ctx:      "ntpd.sys_precision",
		Priority: prioSystemPrecision,
		Dims: collectorapi.Dims{
			{ID: "precision", Name: "precision", Div: precision},
		},
	}
)

var (
	peerChartsTmpl = collectorapi.Charts{
		peerOffsetChartTmpl.Copy(),
		peerDelayChartTmpl.Copy(),
		peerDispersionChartTmpl.Copy(),
		peerJitterChartTmpl.Copy(),
		peerXleaveChartTmpl.Copy(),
		peerRootDelayChartTmpl.Copy(),
		peerRootDispersionChartTmpl.Copy(),
		peerStratumChartTmpl.Copy(),
		peerHostModeChartTmpl.Copy(),
		peerPeerModeChartTmpl.Copy(),
		peerHostPollChartTmpl.Copy(),
		peerPeerPollChartTmpl.Copy(),
		peerPrecisionChartTmpl.Copy(),
	}
	peerOffsetChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_offset",
		Title:    "Peer offset",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_offset",
		Priority: prioPeerOffset,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_offset", Name: "offset", Div: precision},
		},
	}
	peerDelayChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_delay",
		Title:    "Peer delay",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_delay",
		Priority: prioPeerDelay,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_delay", Name: "delay", Div: precision},
		},
	}
	peerDispersionChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_dispersion",
		Title:    "Peer dispersion",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_dispersion",
		Priority: prioPeerDispersion,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_dispersion", Name: "dispersion", Div: precision},
		},
	}
	peerJitterChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_jitter",
		Title:    "Peer jitter",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_jitter",
		Priority: prioPeerJitter,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_jitter", Name: "jitter", Div: precision},
		},
	}
	peerXleaveChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_xleave",
		Title:    "Peer interleave delay",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_xleave",
		Priority: prioPeerXleave,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_xleave", Name: "xleave", Div: precision},
		},
	}
	peerRootDelayChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_rootdelay",
		Title:    "Peer roundtrip delay to the primary reference clock",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_rootdelay",
		Priority: prioPeerRootDelay,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_rootdelay", Name: "rootdelay", Div: precision},
		},
	}
	peerRootDispersionChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_rootdisp",
		Title:    "Peer root dispersion to the primary reference clock",
		Units:    "milliseconds",
		Fam:      "peers",
		Ctx:      "ntpd.peer_rootdisp",
		Priority: prioPeerRootDispersion,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_rootdisp", Name: "dispersion", Div: precision},
		},
	}
	peerStratumChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_stratum",
		Title:    "Peer stratum",
		Units:    "stratum",
		Fam:      "peers",
		Ctx:      "ntpd.peer_stratum",
		Priority: prioPeerStratum,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_stratum", Name: "stratum", Div: precision},
		},
	}
	peerHostModeChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_hmode",
		Title:    "Peer host mode",
		Units:    "hmode",
		Fam:      "peers",
		Ctx:      "ntpd.peer_hmode",
		Priority: prioPeerHostMode,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_hmode", Name: "hmode", Div: precision},
		},
	}
	peerPeerModeChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_pmode",
		Title:    "Peer mode",
		Units:    "pmode",
		Fam:      "peers",
		Ctx:      "ntpd.peer_pmode",
		Priority: prioPeerPeerMode,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_pmode", Name: "pmode", Div: precision},
		},
	}
	peerHostPollChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_hpoll",
		Title:    "Peer host poll exponent",
		Units:    "log2",
		Fam:      "peers",
		Ctx:      "ntpd.peer_hpoll",
		Priority: prioPeerHostPoll,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_hpoll", Name: "hpoll", Div: precision},
		},
	}
	peerPeerPollChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_ppoll",
		Title:    "Peer poll exponent",
		Units:    "log2",
		Fam:      "peers",
		Ctx:      "ntpd.peer_ppoll",
		Priority: prioPeerPeerPoll,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_ppoll", Name: "hpoll", Div: precision},
		},
	}
	peerPrecisionChartTmpl = collectorapi.Chart{
		ID:       "peer_%s_precision",
		Title:    "Peer precision",
		Units:    "log2",
		Fam:      "peers",
		Ctx:      "ntpd.peer_precision",
		Priority: prioPeerPrecision,
		Dims: collectorapi.Dims{
			{ID: "peer_%s_precision", Name: "precision", Div: precision},
		},
	}
)

func (c *Collector) addPeerCharts(addr string) {
	charts := peerChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ReplaceAll(addr, ".", "_"))
		chart.Labels = []collectorapi.Label{
			{Key: "peer_address", Value: addr},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, addr)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removePeerCharts(addr string) {
	px := fmt.Sprintf("peer_%s", strings.ReplaceAll(addr, ".", "_"))

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
