// SPDX-License-Identifier: GPL-3.0-or-later

package typesense

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var baseCharts = module.Charts{
	healthStatusChart.Copy(),
}

const precision = 1000

const (
	prioHealthStatus = module.Priority + iota

	prioTotalRequests
	prioRequestsByOperation
	prioLatencyByOperation
	prioOverloadedRequests
)

var healthStatusChart = module.Chart{
	ID:       "health_status",
	Title:    "Health Status",
	Units:    "status",
	Fam:      "health",
	Ctx:      "typesense.health_status",
	Type:     module.Line,
	Priority: prioHealthStatus,
	Dims: module.Dims{
		{ID: "health_status_ok", Name: "ok"},
		{ID: "health_status_out_of_disk", Name: "out_of_disk"},
		{ID: "health_status_out_of_memory", Name: "out_of_memory"},
	},
}

var statsCharts = module.Charts{
	totalRequestsChart.Copy(),
	requestsByOperationChart.Copy(),
	overloadedRequestsChart.Copy(),
	latencyByOperationChart.Copy(),
}

var (
	totalRequestsChart = module.Chart{
		ID:       "total_requests",
		Title:    "Total Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "typesense.total_requests",
		Type:     module.Line,
		Priority: prioTotalRequests,
		Dims: module.Dims{
			{ID: "total_requests_per_second", Name: "requests", Div: precision},
		},
	}
	requestsByOperationChart = module.Chart{
		ID:       "requests_by_type",
		Title:    "Requests by Operation",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "typesense.requests_by_operation",
		Type:     module.Line,
		Priority: prioRequestsByOperation,
		Dims: module.Dims{
			{ID: "search_requests_per_second", Name: "search", Div: precision},
			{ID: "write_requests_per_second", Name: "write", Div: precision},
			{ID: "import_requests_per_second", Name: "import", Div: precision},
			{ID: "delete_requests_per_second", Name: "delete", Div: precision},
		},
	}
	latencyByOperationChart = module.Chart{
		ID:       "latency_by_operation",
		Title:    "Latency by Operation",
		Units:    "milliseconds",
		Fam:      "requests",
		Ctx:      "typesense.latency_by_operation",
		Type:     module.Line,
		Priority: prioLatencyByOperation,
		Dims: module.Dims{
			{ID: "search_latency_ms", Name: "search"},
			{ID: "write_latency_ms", Name: "write"},
			{ID: "import_latency_ms", Name: "import"},
			{ID: "delete_latency_ms", Name: "delete"},
		},
	}
	overloadedRequestsChart = module.Chart{
		ID:       "overloaded_requests",
		Title:    "Overloaded Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "typesense.overloaded_requests",
		Type:     module.Line,
		Priority: prioOverloadedRequests,
		Dims: module.Dims{
			{ID: "overloaded_requests_per_second", Name: "overloaded", Div: precision},
		},
	}
)

func (c *Collector) addStatsCharts() {
	if err := c.charts.Add(*statsCharts.Copy()...); err != nil {
		c.Warningf("error adding stats charts: %v", err)
	}
}
