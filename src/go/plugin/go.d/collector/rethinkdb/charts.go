// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClusterServersStatsRequest = module.Priority + iota
	prioClusterClientConnections
	prioClusterActiveClients
	prioClusterQueries
	prioClusterDocuments

	prioServerStatsRequestStatus
	prioServerClientConnections
	prioServerActiveClients
	prioServerQueries
	prioServerDocuments
)

var clusterCharts = module.Charts{
	clusterServersStatsRequestChart.Copy(),
	clusterClientConnectionsChart.Copy(),
	clusterActiveClientsChart.Copy(),
	clusterQueriesChart.Copy(),
	clusterDocumentsChart.Copy(),
}

var (
	clusterServersStatsRequestChart = module.Chart{
		ID:       "cluster_cluster_servers_stats_request",
		Title:    "Cluster Servers Stats Request",
		Units:    "servers",
		Fam:      "servers",
		Ctx:      "rethinkdb.cluster_servers_stats_request",
		Priority: prioClusterServersStatsRequest,
		Dims: module.Dims{
			{ID: "cluster_servers_stats_request_success", Name: "success"},
			{ID: "cluster_servers_stats_request_timeout", Name: "timeout"},
		},
	}
	clusterClientConnectionsChart = module.Chart{
		ID:       "cluster_client_connections",
		Title:    "Cluster Client Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "rethinkdb.cluster_client_connections",
		Priority: prioClusterClientConnections,
		Dims: module.Dims{
			{ID: "cluster_client_connections", Name: "connections"},
		},
	}
	clusterActiveClientsChart = module.Chart{
		ID:       "cluster_active_clients",
		Title:    "Cluster Active Clients",
		Units:    "clients",
		Fam:      "clients",
		Ctx:      "rethinkdb.cluster_active_clients",
		Priority: prioClusterActiveClients,
		Dims: module.Dims{
			{ID: "cluster_clients_active", Name: "active"},
		},
	}
	clusterQueriesChart = module.Chart{
		ID:       "cluster_queries",
		Title:    "Cluster Queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "rethinkdb.cluster_queries",
		Priority: prioClusterQueries,
		Dims: module.Dims{
			{ID: "cluster_queries_total", Name: "queries", Algo: module.Incremental},
		},
	}
	clusterDocumentsChart = module.Chart{
		ID:       "cluster_documents",
		Title:    "Cluster Documents",
		Units:    "documents/s",
		Fam:      "documents",
		Ctx:      "rethinkdb.cluster_documents",
		Priority: prioClusterDocuments,
		Dims: module.Dims{
			{ID: "cluster_read_docs_total", Name: "read", Algo: module.Incremental},
			{ID: "cluster_written_docs_total", Name: "written", Mul: -1, Algo: module.Incremental},
		},
	}
)

var serverChartsTmpl = module.Charts{
	serverStatsRequestStatusChartTmpl.Copy(),
	serverConnectionsChartTmpl.Copy(),
	serverActiveClientsChartTmpl.Copy(),
	serverQueriesChartTmpl.Copy(),
	serverDocumentsChartTmpl.Copy(),
}

var (
	serverStatsRequestStatusChartTmpl = module.Chart{
		ID:       "server_%s_stats_request_status",
		Title:    "Server Stats Request Status",
		Units:    "status",
		Fam:      "srv status",
		Ctx:      "rethinkdb.server_stats_request_status",
		Priority: prioServerStatsRequestStatus,
		Dims: module.Dims{
			{ID: "server_%s_stats_request_status_success", Name: "success"},
			{ID: "server_%s_stats_request_status_timeout", Name: "timeout"},
		},
	}
	serverConnectionsChartTmpl = module.Chart{
		ID:       "server_%s_client_connections",
		Title:    "Server Client Connections",
		Units:    "connections",
		Fam:      "srv connections",
		Ctx:      "rethinkdb.server_client_connections",
		Priority: prioServerClientConnections,
		Dims: module.Dims{
			{ID: "server_%s_client_connections", Name: "connections"},
		},
	}
	serverActiveClientsChartTmpl = module.Chart{
		ID:       "server_%s_active_clients",
		Title:    "Server Active Clients",
		Units:    "clients",
		Fam:      "srv clients",
		Ctx:      "rethinkdb.server_active_clients",
		Priority: prioServerActiveClients,
		Dims: module.Dims{
			{ID: "server_%s_clients_active", Name: "active"},
		},
	}
	serverQueriesChartTmpl = module.Chart{
		ID:       "server_%s_queries",
		Title:    "Server Queries",
		Units:    "queries/s",
		Fam:      "srv queries",
		Ctx:      "rethinkdb.server_queries",
		Priority: prioServerQueries,
		Dims: module.Dims{
			{ID: "server_%s_queries_total", Name: "queries", Algo: module.Incremental},
		},
	}
	serverDocumentsChartTmpl = module.Chart{
		ID:       "server_%s_documents",
		Title:    "Server Documents",
		Units:    "documents/s",
		Fam:      "srv documents",
		Ctx:      "rethinkdb.server_documents",
		Priority: prioServerDocuments,
		Dims: module.Dims{
			{ID: "server_%s_read_docs_total", Name: "read", Algo: module.Incremental},
			{ID: "server_%s_written_docs_total", Name: "written", Mul: -1, Algo: module.Incremental},
		},
	}
)

func (c *Collector) addServerCharts(srvUUID, srvName string) {
	charts := serverChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, srvUUID)
		chart.Labels = []module.Label{
			{Key: "sever_uuid", Value: srvUUID},
			{Key: "sever_name", Value: srvName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, srvUUID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add chart for '%s' server: %v", srvName, err)
	}
}

func (c *Collector) removeServerCharts(srvUUID string) {
	px := fmt.Sprintf("server_%s_", srvUUID)
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
