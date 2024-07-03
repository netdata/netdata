// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var chartTmplEntrypointRequests = module.Chart{
	ID:    "entrypoint_requests_%s_%s",
	Title: "Processed HTTP requests on <code>%s</code> entrypoint (protocol <code>%s</code>)",
	Units: "requests/s",
	Fam:   "entrypoint %s %s",
	Ctx:   "traefik.entrypoint_requests",
	Type:  module.Stacked,
	Dims: module.Dims{
		{ID: prefixEntrypointRequests + "%s_%s_1xx", Name: "1xx", Algo: module.Incremental},
		{ID: prefixEntrypointRequests + "%s_%s_2xx", Name: "2xx", Algo: module.Incremental},
		{ID: prefixEntrypointRequests + "%s_%s_3xx", Name: "3xx", Algo: module.Incremental},
		{ID: prefixEntrypointRequests + "%s_%s_4xx", Name: "4xx", Algo: module.Incremental},
		{ID: prefixEntrypointRequests + "%s_%s_5xx", Name: "5xx", Algo: module.Incremental},
	},
}

var chartTmplEntrypointRequestDuration = module.Chart{
	ID:    "entrypoint_request_duration_%s_%s",
	Title: "Average HTTP request processing time on <code>%s</code> entrypoint (protocol <code>%s</code>)",
	Units: "milliseconds",
	Fam:   "entrypoint %s %s",
	Ctx:   "traefik.entrypoint_request_duration_average",
	Type:  module.Stacked,
	Dims: module.Dims{
		{ID: prefixEntrypointReqDurAvg + "%s_%s_1xx", Name: "1xx"},
		{ID: prefixEntrypointReqDurAvg + "%s_%s_2xx", Name: "2xx"},
		{ID: prefixEntrypointReqDurAvg + "%s_%s_3xx", Name: "3xx"},
		{ID: prefixEntrypointReqDurAvg + "%s_%s_4xx", Name: "4xx"},
		{ID: prefixEntrypointReqDurAvg + "%s_%s_5xx", Name: "5xx"},
	},
}

var chartTmplEntrypointOpenConnections = module.Chart{
	ID:    "entrypoint_open_connections_%s_%s",
	Title: "Open connections on <code>%s</code> entrypoint (protocol <code>%s</code>)",
	Units: "connections",
	Fam:   "entrypoint %s %s",
	Ctx:   "traefik.entrypoint_open_connections",
	Type:  module.Stacked,
}

func newChartEntrypointRequests(entrypoint, proto string) *module.Chart {
	return newEntrypointChart(chartTmplEntrypointRequests, entrypoint, proto)
}

func newChartEntrypointRequestDuration(entrypoint, proto string) *module.Chart {
	return newEntrypointChart(chartTmplEntrypointRequestDuration, entrypoint, proto)
}

func newChartEntrypointOpenConnections(entrypoint, proto string) *module.Chart {
	return newEntrypointChart(chartTmplEntrypointOpenConnections, entrypoint, proto)
}

func newEntrypointChart(tmpl module.Chart, entrypoint, proto string) *module.Chart {
	chart := tmpl.Copy()
	chart.ID = fmt.Sprintf(chart.ID, entrypoint, proto)
	chart.Title = fmt.Sprintf(chart.Title, entrypoint, proto)
	chart.Fam = fmt.Sprintf(chart.Fam, entrypoint, proto)
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, entrypoint, proto)
	}
	return chart
}
