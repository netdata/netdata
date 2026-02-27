// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioResponseTime = collectorapi.Priority + iota
	prioResponseLength
	prioResponseStatus
	prioResponseInStatusDuration
)

var httpCheckCharts = collectorapi.Charts{
	responseTimeChart.Copy(),
	responseLengthChart.Copy(),
	responseStatusChart.Copy(),
	responseInStatusDurationChart.Copy(),
}

var responseTimeChart = collectorapi.Chart{
	ID:       "response_time",
	Title:    "HTTP Response Time",
	Units:    "ms",
	Fam:      "response",
	Ctx:      "httpcheck.response_time",
	Priority: prioResponseTime,
	Dims: collectorapi.Dims{
		{ID: "time"},
	},
}

var responseLengthChart = collectorapi.Chart{
	ID:       "response_length",
	Title:    "HTTP Response Body Length",
	Units:    "characters",
	Fam:      "response",
	Ctx:      "httpcheck.response_length",
	Priority: prioResponseLength,
	Dims: collectorapi.Dims{
		{ID: "length"},
	},
}

var responseStatusChart = collectorapi.Chart{
	ID:       "request_status",
	Title:    "HTTP Check Status",
	Units:    "boolean",
	Fam:      "status",
	Ctx:      "httpcheck.status",
	Priority: prioResponseStatus,
	Dims: collectorapi.Dims{
		{ID: "success"},
		{ID: "no_connection"},
		{ID: "timeout"},
		{ID: "redirect"},
		{ID: "bad_content"},
		{ID: "bad_status"},
		{ID: "bad_header"},
	},
}

var responseInStatusDurationChart = collectorapi.Chart{
	ID:       "current_state_duration",
	Title:    "HTTP Current State Duration",
	Units:    "seconds",
	Fam:      "status",
	Ctx:      "httpcheck.in_state",
	Priority: prioResponseInStatusDuration,
	Dims: collectorapi.Dims{
		{ID: "in_state", Name: "time"},
	},
}
