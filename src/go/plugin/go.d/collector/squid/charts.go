// SPDX-License-Identifier: GPL-3.0-or-later

package squid

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioClientsNet = collectorapi.Priority + iota
	prioClientsRequests
	prioServersNet
	prioServersRequests
)

var charts = collectorapi.Charts{
	clientsNetChart.Copy(),
	clientsRequestsChart.Copy(),
	serversNetChart.Copy(),
	serversRequestsChart.Copy(),
}

var (
	clientsNetChart = collectorapi.Chart{
		ID:       "clients_net",
		Title:    "Squid Client Bandwidth",
		Units:    "kilobits/s",
		Fam:      "clients",
		Ctx:      "squid.clients_net",
		Type:     collectorapi.Area,
		Priority: prioClientsNet,
		Dims: collectorapi.Dims{
			{ID: "client_http.kbytes_in", Name: "in", Algo: collectorapi.Incremental, Mul: 8},
			{ID: "client_http.kbytes_out", Name: "out", Algo: collectorapi.Incremental, Mul: -8},
			{ID: "client_http.hit_kbytes_out", Name: "hits", Algo: collectorapi.Incremental, Mul: -8},
		},
	}

	clientsRequestsChart = collectorapi.Chart{
		ID:       "clients_requests",
		Title:    "Squid Client Requests",
		Units:    "requests/s",
		Fam:      "clients",
		Ctx:      "squid.clients_requests",
		Type:     collectorapi.Line,
		Priority: prioClientsRequests,
		Dims: collectorapi.Dims{
			{ID: "client_http.requests", Name: "requests", Algo: collectorapi.Incremental},
			{ID: "client_http.hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "client_http.errors", Name: "errors", Algo: collectorapi.Incremental, Mul: -1},
		},
	}

	serversNetChart = collectorapi.Chart{
		ID:       "servers_net",
		Title:    "Squid Server Bandwidth",
		Units:    "kilobits/s",
		Fam:      "servers",
		Ctx:      "squid.servers_net",
		Type:     collectorapi.Area,
		Priority: prioServersNet,
		Dims: collectorapi.Dims{
			{ID: "server.all.kbytes_in", Name: "in", Algo: collectorapi.Incremental, Mul: 8},
			{ID: "server.all.kbytes_out", Name: "out", Algo: collectorapi.Incremental, Mul: -8},
		},
	}

	serversRequestsChart = collectorapi.Chart{
		ID:       "servers_requests",
		Title:    "Squid Server Requests",
		Units:    "requests/s",
		Fam:      "servers",
		Ctx:      "squid.servers_requests",
		Type:     collectorapi.Line,
		Priority: prioServersRequests,
		Dims: collectorapi.Dims{
			{ID: "server.all.requests", Name: "requests", Algo: collectorapi.Incremental},
			{ID: "server.all.errors", Name: "errors", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
)
