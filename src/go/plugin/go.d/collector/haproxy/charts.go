// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var charts = collectorapi.Charts{
	chartBackendCurrentSessions.Copy(),
	chartBackendSessions.Copy(),

	chartBackendResponseTimeAverage.Copy(),

	chartBackendQueueTimeAverage.Copy(),
	chartBackendCurrentQueue.Copy(),
}

var (
	chartBackendCurrentSessions = collectorapi.Chart{
		ID:    "backend_current_sessions",
		Title: "Current number of active sessions",
		Units: "sessions",
		Fam:   "backend sessions",
		Ctx:   "haproxy.backend_current_sessions",
	}
	chartBackendSessions = collectorapi.Chart{
		ID:    "backend_sessions",
		Title: "Sessions rate",
		Units: "sessions/s",
		Fam:   "backend sessions",
		Ctx:   "haproxy.backend_sessions",
	}
)

var (
	chartBackendResponseTimeAverage = collectorapi.Chart{
		ID:    "backend_response_time_average",
		Title: "Average response time for last 1024 successful connections",
		Units: "milliseconds",
		Fam:   "backend responses",
		Ctx:   "haproxy.backend_response_time_average",
	}
	chartTemplateBackendHTTPResponses = collectorapi.Chart{
		ID:    "backend_http_responses_proxy_%s",
		Title: "HTTP responses by code class for <code>%s</code> proxy",
		Units: "responses/s",
		Fam:   "backend responses",
		Ctx:   "haproxy.backend_http_responses",
		Type:  collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "haproxy_backend_http_responses_1xx_proxy_%s", Name: "1xx", Algo: collectorapi.Incremental},
			{ID: "haproxy_backend_http_responses_2xx_proxy_%s", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "haproxy_backend_http_responses_3xx_proxy_%s", Name: "3xx", Algo: collectorapi.Incremental},
			{ID: "haproxy_backend_http_responses_4xx_proxy_%s", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "haproxy_backend_http_responses_5xx_proxy_%s", Name: "5xx", Algo: collectorapi.Incremental},
			{ID: "haproxy_backend_http_responses_other_proxy_%s", Name: "other", Algo: collectorapi.Incremental},
		},
	}
)

var (
	chartBackendQueueTimeAverage = collectorapi.Chart{
		ID:    "backend_queue_time_average",
		Title: "Average queue time for last 1024 successful connections",
		Units: "milliseconds",
		Fam:   "backend queue",
		Ctx:   "haproxy.backend_queue_time_average",
	}
	chartBackendCurrentQueue = collectorapi.Chart{
		ID:    "backend_current_queue",
		Title: "Current number of queued requests",
		Units: "requests",
		Fam:   "backend queue",
		Ctx:   "haproxy.backend_current_queue",
	}
)

var (
	chartTemplateBackendNetworkIO = collectorapi.Chart{
		ID:    "backend_network_io_proxy_%s",
		Title: "Network traffic for <code>%s</code> proxy",
		Units: "bytes/s",
		Fam:   "backend network",
		Ctx:   "haproxy.backend_network_io",
		Type:  collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "haproxy_backend_bytes_in_proxy_%s", Name: "in", Algo: collectorapi.Incremental},
			{ID: "haproxy_backend_bytes_out_proxy_%s", Name: "out", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
)

func newChartBackendHTTPResponses(proxy string) *collectorapi.Chart {
	return newBackendChartFromTemplate(chartTemplateBackendHTTPResponses, proxy)
}

func newChartBackendNetworkIO(proxy string) *collectorapi.Chart {
	return newBackendChartFromTemplate(chartTemplateBackendNetworkIO, proxy)
}

func newBackendChartFromTemplate(tpl collectorapi.Chart, proxy string) *collectorapi.Chart {
	c := tpl.Copy()
	c.ID = fmt.Sprintf(c.ID, proxy)
	c.Title = fmt.Sprintf(c.Title, proxy)
	for _, d := range c.Dims {
		d.ID = fmt.Sprintf(d.ID, proxy)
	}
	return c
}
