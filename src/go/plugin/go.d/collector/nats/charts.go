// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioServerTraffic = module.Priority + iota
	prioServerMessages
	prioServerConnectionsCurrent
	prioServerConnectionsRate
	prioHttpEndpointRequests
	prioServerHealthProbeStatus
	prioServerCpuUsage
	prioServerMemoryUsage
	prioServerUptime
)

var serverCharts = func() module.Charts {
	charts := module.Charts{
		chartServerConnectionsCurrent.Copy(),
		chartServerConnectionsRate.Copy(),
		chartServerTraffic.Copy(),
		chartServerMessages.Copy(),
		chartServerHealthProbeStatus.Copy(),
		chartServerCpuUsage.Copy(),
		chartServerMemUsage.Copy(),
		chartServerUptime.Copy(),
	}
	charts = append(charts, httpEndpointCharts()...)
	return charts
}()

var (
	chartServerTraffic = module.Chart{
		ID:       "server_traffic",
		Title:    "Server Traffic",
		Units:    "bytes/s",
		Fam:      "traffic",
		Ctx:      "nats.server_traffic",
		Priority: prioServerTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "in_bytes", Name: "in", Algo: module.Incremental},
			{ID: "out_bytes", Name: "out", Mul: -1, Algo: module.Incremental},
		},
	}
	chartServerMessages = module.Chart{
		ID:       "server_messages",
		Title:    "Server Messages",
		Units:    "messages/s",
		Fam:      "traffic",
		Ctx:      "nats.server_messages",
		Priority: prioServerMessages,
		Dims: module.Dims{
			{ID: "in_msgs", Name: "in", Algo: module.Incremental},
			{ID: "out_msgs", Name: "out", Mul: -1, Algo: module.Incremental},
		},
	}
	chartServerConnectionsCurrent = module.Chart{
		ID:       "server_connections_current",
		Title:    "Server Current Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "nats.server_connections_current",
		Priority: prioServerConnectionsCurrent,
		Dims: module.Dims{
			{ID: "connections", Name: "active"},
		},
	}
	chartServerConnectionsRate = module.Chart{
		ID:       "server_connections_rate",
		Title:    "Server Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "nats.server_connections_rate",
		Priority: prioServerConnectionsRate,
		Dims: module.Dims{
			{ID: "total_connections", Name: "connections", Algo: module.Incremental},
		},
	}
	chartServerHealthProbeStatus = module.Chart{
		ID:       "server_health_probe_status",
		Title:    "Server Health Probe Status",
		Units:    "status",
		Fam:      "health",
		Ctx:      "nats.server_health_probe_status",
		Priority: prioServerHealthProbeStatus,
		Dims: module.Dims{
			{ID: "healthz_status_ok", Name: "ok"},
			{ID: "healthz_status_error", Name: "error"},
		},
	}
	chartServerCpuUsage = module.Chart{
		ID:       "server_cpu_usage",
		Title:    "Server CPU Usage",
		Units:    "percent",
		Fam:      "rusage",
		Ctx:      "nats.server_cpu_usage",
		Priority: prioServerCpuUsage,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "cpu", Name: "used"},
		},
	}
	chartServerMemUsage = module.Chart{
		ID:       "server_mem_usage",
		Title:    "Server Memory Usage",
		Units:    "bytes",
		Fam:      "rusage",
		Ctx:      "nats.server_mem_usage",
		Priority: prioServerMemoryUsage,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "mem", Name: "used"},
		},
	}
	chartServerUptime = module.Chart{
		ID:       "server_uptime",
		Title:    "Server Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "nats.server_uptime",
		Priority: prioServerUptime,
		Dims: module.Dims{
			{ID: "uptime", Name: "uptime"},
		},
	}
)

func httpEndpointCharts() module.Charts {
	var charts module.Charts
	for _, path := range httpEndpoints {
		chart := httpEndpointRequestsChartTmpl.Copy()
		chart.ID = fmt.Sprintf(chart.ID, path)
		chart.Labels = []module.Label{
			{Key: "http_endpoint", Value: path},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, path)
		}
		charts = append(charts, chart)
	}
	return charts
}

var httpEndpointRequestsChartTmpl = module.Chart{
	ID:       "http_endpoint_%s_requests",
	Title:    "HTTP Endpoint Requests",
	Units:    "requests/s",
	Fam:      "http requests",
	Ctx:      "nats.http_endpoint_requests",
	Priority: prioHttpEndpointRequests,
	Dims: module.Dims{
		{ID: "http_endpoint_%s_req", Name: "requests", Algo: module.Incremental},
	},
}
