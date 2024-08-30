// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var charts = module.Charts{
	chartBackendCurrentSessions.Copy(),
	chartBackendSessions.Copy(),

	chartBackendResponseTimeAverage.Copy(),

	chartBackendQueueTimeAverage.Copy(),
	chartBackendCurrentQueue.Copy(),
}

var (
	chartBackendCurrentSessions = module.Chart{
		ID:    "backend_current_sessions",
		Title: "Current number of active sessions",
		Units: "sessions",
		Fam:   "backend sessions",
		Ctx:   "haproxy.backend_current_sessions",
	}
	chartBackendSessions = module.Chart{
		ID:    "backend_sessions",
		Title: "Sessions rate",
		Units: "sessions/s",
		Fam:   "backend sessions",
		Ctx:   "haproxy.backend_sessions",
	}
)

var (
	chartBackendResponseTimeAverage = module.Chart{
		ID:    "backend_response_time_average",
		Title: "Average response time for last 1024 successful connections",
		Units: "milliseconds",
		Fam:   "backend responses",
		Ctx:   "haproxy.backend_response_time_average",
	}
	chartTemplateBackendHTTPResponses = module.Chart{
		ID:    "backend_http_responses_proxy_%s",
		Title: "HTTP responses by code class for <code>%s</code> proxy",
		Units: "responses/s",
		Fam:   "backend responses",
		Ctx:   "haproxy.backend_http_responses",
		Type:  module.Stacked,
		Dims: module.Dims{
			{ID: "haproxy_backend_http_responses_1xx_proxy_%s", Name: "1xx", Algo: module.Incremental},
			{ID: "haproxy_backend_http_responses_2xx_proxy_%s", Name: "2xx", Algo: module.Incremental},
			{ID: "haproxy_backend_http_responses_3xx_proxy_%s", Name: "3xx", Algo: module.Incremental},
			{ID: "haproxy_backend_http_responses_4xx_proxy_%s", Name: "4xx", Algo: module.Incremental},
			{ID: "haproxy_backend_http_responses_5xx_proxy_%s", Name: "5xx", Algo: module.Incremental},
			{ID: "haproxy_backend_http_responses_other_proxy_%s", Name: "other", Algo: module.Incremental},
		},
	}
)

var (
	chartBackendQueueTimeAverage = module.Chart{
		ID:    "backend_queue_time_average",
		Title: "Average queue time for last 1024 successful connections",
		Units: "milliseconds",
		Fam:   "backend queue",
		Ctx:   "haproxy.backend_queue_time_average",
	}
	chartBackendCurrentQueue = module.Chart{
		ID:    "backend_current_queue",
		Title: "Current number of queued requests",
		Units: "requests",
		Fam:   "backend queue",
		Ctx:   "haproxy.backend_current_queue",
	}
)

var (
	chartTemplateBackendNetworkIO = module.Chart{
		ID:    "backend_network_io_proxy_%s",
		Title: "Network traffic for <code>%s</code> proxy",
		Units: "bytes/s",
		Fam:   "backend network",
		Ctx:   "haproxy.backend_network_io",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "haproxy_backend_bytes_in_proxy_%s", Name: "in", Algo: module.Incremental},
			{ID: "haproxy_backend_bytes_out_proxy_%s", Name: "out", Algo: module.Incremental, Mul: -1},
		},
	}
)

func newChartBackendHTTPResponses(proxy string) *module.Chart {
	return newBackendChartFromTemplate(chartTemplateBackendHTTPResponses, proxy)
}

func newChartBackendNetworkIO(proxy string) *module.Chart {
	return newBackendChartFromTemplate(chartTemplateBackendNetworkIO, proxy)
}

func newBackendChartFromTemplate(tpl module.Chart, proxy string) *module.Chart {
	c := tpl.Copy()
	c.ID = fmt.Sprintf(c.ID, proxy)
	c.Title = fmt.Sprintf(c.Title, proxy)
	for _, d := range c.Dims {
		d.ID = fmt.Sprintf(d.ID, proxy)
	}
	return c
}
