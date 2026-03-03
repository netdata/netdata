// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
)

var charts = Charts{
	{
		ID:    "bandwidth_total",
		Title: "Bandwidth",
		Units: "B/s",
		Fam:   "bandwidth",
		Ctx:   "tengine.bandwidth_total",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: "bytes_in", Name: "in", Algo: collectorapi.Incremental},
			{ID: "bytes_out", Name: "out", Algo: collectorapi.Incremental, Mul: -1},
		},
	},
	{
		ID:    "connections_total",
		Title: "Connections",
		Units: "connections/s",
		Fam:   "connections",
		Ctx:   "tengine.connections_total",
		Dims: Dims{
			{ID: "conn_total", Name: "accepted", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "requests_total",
		Title: "Requests",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "tengine.requests_total",
		Dims: Dims{
			{ID: "req_total", Name: "processed", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "requests_per_response_code_family_total",
		Title: "Requests Per Response Code Family",
		Units: "requests/s",
		Fam:   "requests",
		Ctx:   "tengine.requests_per_response_code_family_total",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: "http_2xx", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "http_5xx", Name: "5xx", Algo: collectorapi.Incremental},
			{ID: "http_3xx", Name: "3xx", Algo: collectorapi.Incremental},
			{ID: "http_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "http_other_status", Name: "other", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "requests_per_response_code_detailed_total",
		Title: "Requests Per Response Code Detailed",
		Units: "requests/s",
		Ctx:   "tengine.requests_per_response_code_detailed_total",
		Fam:   "requests",
		Type:  collectorapi.Stacked,
		Dims: Dims{
			{ID: "http_200", Name: "200", Algo: collectorapi.Incremental},
			{ID: "http_206", Name: "206", Algo: collectorapi.Incremental},
			{ID: "http_302", Name: "302", Algo: collectorapi.Incremental},
			{ID: "http_304", Name: "304", Algo: collectorapi.Incremental},
			{ID: "http_403", Name: "403", Algo: collectorapi.Incremental},
			{ID: "http_404", Name: "404", Algo: collectorapi.Incremental},
			{ID: "http_416", Name: "419", Algo: collectorapi.Incremental},
			{ID: "http_499", Name: "499", Algo: collectorapi.Incremental},
			{ID: "http_500", Name: "500", Algo: collectorapi.Incremental},
			{ID: "http_502", Name: "502", Algo: collectorapi.Incremental},
			{ID: "http_503", Name: "503", Algo: collectorapi.Incremental},
			{ID: "http_504", Name: "504", Algo: collectorapi.Incremental},
			{ID: "http_508", Name: "508", Algo: collectorapi.Incremental},
			{ID: "http_other_detail_status", Name: "other", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "requests_upstream_total",
		Title: "Number Of Requests Calling For Upstream",
		Units: "requests/s",
		Fam:   "upstream",
		Ctx:   "tengine.requests_upstream_total",
		Dims: Dims{
			{ID: "ups_req", Name: "requests", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "tries_upstream_total",
		Title: "Number Of Times Calling For Upstream",
		Units: "calls/s",
		Fam:   "upstream",
		Ctx:   "tengine.tries_upstream_total",
		Dims: Dims{
			{ID: "ups_tries", Name: "calls", Algo: collectorapi.Incremental},
		},
	},
	{
		ID:    "requests_upstream_per_response_code_family_total",
		Title: "Upstream Requests Per Response Code Family",
		Units: "requests/s",
		Fam:   "upstream",
		Type:  collectorapi.Stacked,
		Ctx:   "tengine.requests_upstream_per_response_code_family_total",
		Dims: Dims{
			{ID: "http_ups_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "http_ups_5xx", Name: "5xx", Algo: collectorapi.Incremental},
		},
	},
}
