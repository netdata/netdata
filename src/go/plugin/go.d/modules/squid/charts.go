// SPDX-License-Identifier: GPL-3.0-or-later

package squid

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClientsNet = module.Priority + iota
	prioClientsRequests
	prioServersNet
	prioServersRequests
)

var charts = module.Charts{
	clientsNetChart.Copy(),
	clientsRequestsChart.Copy(),
	serversNetChart.Copy(),
	serversRequestsChart.Copy(),
}

var (
	clientsNetChart = module.Chart{
		ID:       "clients_net",
		Title:    "Squid Client Bandwidth",
		Units:    "kilobits/s",
		Fam:      "clients",
		Ctx:      "squid.clients_net",
		Type:     module.Area,
		Priority: prioClientsNet,
		Dims: module.Dims{
			{ID: "client_http.kbytes_in", Name: "in", Algo: module.Incremental, Mul: 8},
			{ID: "client_http.kbytes_out", Name: "out", Algo: module.Incremental, Mul: -8},
			{ID: "client_http.hit_kbytes_out", Name: "hits", Algo: module.Incremental, Mul: -8},
		},
	}

	clientsRequestsChart = module.Chart{
		ID:       "clients_requests",
		Title:    "Squid Client Requests",
		Units:    "requests/s",
		Fam:      "clients",
		Ctx:      "squid.clients_requests",
		Type:     module.Line,
		Priority: prioClientsRequests,
		Dims: module.Dims{
			{ID: "client_http.requests", Name: "requests", Algo: module.Incremental},
			{ID: "client_http.hits", Name: "hits", Algo: module.Incremental},
			{ID: "client_http.errors", Name: "errors", Algo: module.Incremental, Mul: -1},
		},
	}

	serversNetChart = module.Chart{
		ID:       "servers_net",
		Title:    "Squid Server Bandwidth",
		Units:    "kilobits/s",
		Fam:      "servers",
		Ctx:      "squid.servers_net",
		Type:     module.Area,
		Priority: prioServersNet,
		Dims: module.Dims{
			{ID: "server.all.kbytes_in", Name: "in", Algo: module.Incremental, Mul: 8},
			{ID: "server.all.kbytes_out", Name: "out", Algo: module.Incremental, Mul: -8},
		},
	}

	serversRequestsChart = module.Chart{
		ID:       "servers_requests",
		Title:    "Squid Server Requests",
		Units:    "requests/s",
		Fam:      "servers",
		Ctx:      "squid.servers_requests",
		Type:     module.Line,
		Priority: prioServersRequests,
		Dims: module.Dims{
			{ID: "server.all.requests", Name: "requests", Algo: module.Incremental},
			{ID: "server.all.errors", Name: "errors", Algo: module.Incremental, Mul: -1},
		},
	}
)
