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
)

var chartsTmpl = module.Charts{
	checkStatusChartTmpl.Copy(),
	checkInStateDurationChartTmpl.Copy(),
	checkConnectionLatencyChartTmpl.Copy(),
}

var checkStatusChartTmpl = module.Chart{
	ID:       "port_%d_status",
	Title:    "TCP Check Status",
	Units:    "boolean",
	Fam:      "status",
	Ctx:      "portcheck.status",
	Priority: prioCheckStatus,
	Dims: module.Dims{
		{ID: "port_%d_success", Name: "success"},
		{ID: "port_%d_failed", Name: "failed"},
		{ID: "port_%d_timeout", Name: "timeout"},
	},
}

var checkInStateDurationChartTmpl = module.Chart{
	ID:       "port_%d_current_state_duration",
	Title:    "Current State Duration",
	Units:    "seconds",
	Fam:      "status duration",
	Ctx:      "portcheck.state_duration",
	Priority: prioCheckInStatusDuration,
	Dims: module.Dims{
		{ID: "port_%d_current_state_duration", Name: "time"},
	},
}

var checkConnectionLatencyChartTmpl = module.Chart{
	ID:       "port_%d_connection_latency",
	Title:    "TCP Connection Latency",
	Units:    "ms",
	Fam:      "latency",
	Ctx:      "portcheck.latency",
	Priority: prioCheckLatency,
	Dims: module.Dims{
		{ID: "port_%d_latency", Name: "time"},
	},
}

func newPortCharts(host string, port int) *module.Charts {
	charts := chartsTmpl.Copy()
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
