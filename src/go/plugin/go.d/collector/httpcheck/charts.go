// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioResponseTime = module.Priority + iota
	prioResponseLength
	prioResponseStatus
	prioResponseInStatusDuration
)

var httpCheckCharts = module.Charts{
	responseTimeChart.Copy(),
	responseLengthChart.Copy(),
	responseStatusChart.Copy(),
	responseInStatusDurationChart.Copy(),
}

var responseTimeChart = module.Chart{
	ID:       "response_time",
	Title:    "HTTP Response Time",
	Units:    "ms",
	Fam:      "response",
	Ctx:      "httpcheck.response_time",
	Priority: prioResponseTime,
	Dims: module.Dims{
		{ID: "time"},
	},
}

var responseLengthChart = module.Chart{
	ID:       "response_length",
	Title:    "HTTP Response Body Length",
	Units:    "characters",
	Fam:      "response",
	Ctx:      "httpcheck.response_length",
	Priority: prioResponseLength,
	Dims: module.Dims{
		{ID: "length"},
	},
}

var responseStatusChart = module.Chart{
	ID:       "request_status",
	Title:    "HTTP Check Status",
	Units:    "boolean",
	Fam:      "status",
	Ctx:      "httpcheck.status",
	Priority: prioResponseStatus,
	Dims: module.Dims{
		{ID: "success"},
		{ID: "no_connection"},
		{ID: "timeout"},
		{ID: "redirect"},
		{ID: "bad_content"},
		{ID: "bad_status"},
		{ID: "bad_header"},
	},
}

var responseInStatusDurationChart = module.Chart{
	ID:       "current_state_duration",
	Title:    "HTTP Current State Duration",
	Units:    "seconds",
	Fam:      "status",
	Ctx:      "httpcheck.in_state",
	Priority: prioResponseInStatusDuration,
	Dims: module.Dims{
		{ID: "in_state", Name: "time"},
	},
}
