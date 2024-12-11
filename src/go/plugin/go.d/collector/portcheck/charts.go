// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioCheckStatus = module.Priority + iota
	prioCheckInStatusDuration
	prioCheckLatency

	prioUDPCheckStatus
	prioUDPCheckInStatusDuration
)

var tcpPortChartsTmpl = module.Charts{
	tcpPortCheckStatusChartTmpl.Copy(),
	tcpPortCheckInStateDurationChartTmpl.Copy(),
	tcpPortCheckConnectionLatencyChartTmpl.Copy(),
}

var udpPortChartsTmpl = module.Charts{
	udpPortCheckStatusChartTmpl.Copy(),
	udpPortCheckInStatusDurationChartTmpl.Copy(),
}

var (
	tcpPortCheckStatusChartTmpl = module.Chart{
		ID:       "port_%d_status",
		Title:    "TCP Check Status",
		Units:    "boolean",
		Fam:      "status",
		Ctx:      "portcheck.status",
		Priority: prioCheckStatus,
		Dims: module.Dims{
			{ID: "tcp_port_%d_success", Name: "success"},
			{ID: "tcp_port_%d_failed", Name: "failed"},
			{ID: "tcp_port_%d_timeout", Name: "timeout"},
		},
	}
	tcpPortCheckInStateDurationChartTmpl = module.Chart{
		ID:       "port_%d_current_state_duration",
		Title:    "Current State Duration",
		Units:    "seconds",
		Fam:      "status duration",
		Ctx:      "portcheck.state_duration",
		Priority: prioCheckInStatusDuration,
		Dims: module.Dims{
			{ID: "tcp_port_%d_current_state_duration", Name: "time"},
		},
	}
	tcpPortCheckConnectionLatencyChartTmpl = module.Chart{
		ID:       "port_%d_connection_latency",
		Title:    "TCP Connection Latency",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "portcheck.latency",
		Priority: prioCheckLatency,
		Dims: module.Dims{
			{ID: "tcp_port_%d_latency", Name: "time"},
		},
	}
)

var (
	udpPortCheckStatusChartTmpl = module.Chart{
		ID:       "udp_port_%d_check_status",
		Title:    "UDP Port Check Status",
		Units:    "status",
		Fam:      "status",
		Ctx:      "portcheck.udp_port_status",
		Priority: prioUDPCheckStatus,
		Dims: module.Dims{
			{ID: "udp_port_%d_open_filtered", Name: "open/filtered"},
			{ID: "udp_port_%d_closed", Name: "closed"},
		},
	}
	udpPortCheckInStatusDurationChartTmpl = module.Chart{
		ID:       "udp_port_%d_current_status_duration",
		Title:    "UDP Port Current Status Duration",
		Units:    "seconds",
		Fam:      "status duration",
		Ctx:      "portcheck.udp_port_status_duration",
		Priority: prioUDPCheckInStatusDuration,
		Dims: module.Dims{
			{ID: "udp_port_%d_current_status_duration", Name: "time"},
		},
	}
)

func (c *Collector) addTCPPortCharts(port *tcpPort) {
	charts := newPortCharts(c.Host, port.number, tcpPortChartsTmpl.Copy())

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addUDPPortCharts(port *udpPort) {
	charts := newPortCharts(c.Host, port.number, udpPortChartsTmpl.Copy())

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func newPortCharts(host string, port int, charts *module.Charts) *module.Charts {
	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "host", Value: host},
			{Key: "port", Value: strconv.Itoa(port)},
		}
		chart.ID = fmt.Sprintf(chart.ID, port)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, port)
		}
	}
	return charts
}
