// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClientConnectionsUtilization = module.Priority + iota
	prioDBClientConnections
	prioDBServerConnections
	prioDBServerConnectionsUtilization
	prioDBClientsWaitTime
	prioDBClientsWaitMaxTime
	prioDBTransactions
	prioDBTransactionsTime
	prioDBTransactionsAvgTime
	prioDBQueries
	prioDBQueriesTime
	prioDBQueryAvgTime
	prioDBNetworkIO
)

var (
	globalCharts = module.Charts{
		clientConnectionsUtilization.Copy(),
	}

	clientConnectionsUtilization = module.Chart{
		ID:       "client_connections_utilization",
		Title:    "Client connections utilization",
		Units:    "percentage",
		Fam:      "client connections",
		Ctx:      "pgbouncer.client_connections_utilization",
		Priority: prioClientConnectionsUtilization,
		Dims: module.Dims{
			{ID: "cl_conns_utilization", Name: "used"},
		},
	}
)

var (
	dbChartsTmpl = module.Charts{
		dbClientConnectionsTmpl.Copy(),

		dbServerConnectionsUtilizationTmpl.Copy(),
		dbServerConnectionsTmpl.Copy(),

		dbClientsWaitTimeChartTmpl.Copy(),
		dbClientMaxWaitTimeChartTmpl.Copy(),

		dbTransactionsChartTmpl.Copy(),
		dbTransactionsTimeChartTmpl.Copy(),
		dbTransactionAvgTimeChartTmpl.Copy(),

		dbQueriesChartTmpl.Copy(),
		dbQueriesTimeChartTmpl.Copy(),
		dbQueryAvgTimeChartTmpl.Copy(),

		dbNetworkIOChartTmpl.Copy(),
	}

	dbClientConnectionsTmpl = module.Chart{
		ID:       "db_%s_client_connections",
		Title:    "Database client connections",
		Units:    "connections",
		Fam:      "client connections",
		Ctx:      "pgbouncer.db_client_connections",
		Priority: prioDBClientConnections,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "db_%s_cl_active", Name: "active"},
			{ID: "db_%s_cl_waiting", Name: "waiting"},
			{ID: "db_%s_cl_cancel_req", Name: "cancel_req"},
		},
	}

	dbServerConnectionsTmpl = module.Chart{
		ID:       "db_%s_server_connections",
		Title:    "Database server connections",
		Units:    "connections",
		Fam:      "server connections",
		Ctx:      "pgbouncer.db_server_connections",
		Priority: prioDBServerConnections,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "db_%s_sv_active", Name: "active"},
			{ID: "db_%s_sv_idle", Name: "idle"},
			{ID: "db_%s_sv_used", Name: "used"},
			{ID: "db_%s_sv_tested", Name: "tested"},
			{ID: "db_%s_sv_login", Name: "login"},
		},
	}

	dbServerConnectionsUtilizationTmpl = module.Chart{
		ID:       "db_%s_server_connections_utilization",
		Title:    "Database server connections utilization",
		Units:    "percentage",
		Fam:      "server connections limit",
		Ctx:      "pgbouncer.db_server_connections_utilization",
		Priority: prioDBServerConnectionsUtilization,
		Dims: module.Dims{
			{ID: "db_%s_sv_conns_utilization", Name: "used"},
		},
	}

	dbClientsWaitTimeChartTmpl = module.Chart{
		ID:       "db_%s_clients_wait_time",
		Title:    "Database clients wait time",
		Units:    "seconds",
		Fam:      "clients wait time",
		Ctx:      "pgbouncer.db_clients_wait_time",
		Priority: prioDBClientsWaitTime,
		Dims: module.Dims{
			{ID: "db_%s_total_wait_time", Name: "time", Algo: module.Incremental, Div: 1e6},
		},
	}
	dbClientMaxWaitTimeChartTmpl = module.Chart{
		ID:       "db_%s_client_max_wait_time",
		Title:    "Database client max wait time",
		Units:    "seconds",
		Fam:      "client max wait time",
		Ctx:      "pgbouncer.db_client_max_wait_time",
		Priority: prioDBClientsWaitMaxTime,
		Dims: module.Dims{
			{ID: "db_%s_maxwait", Name: "time", Div: 1e6},
		},
	}

	dbTransactionsChartTmpl = module.Chart{
		ID:       "db_%s_transactions",
		Title:    "Database pooled SQL transactions",
		Units:    "transactions/s",
		Fam:      "transactions",
		Ctx:      "pgbouncer.db_transactions",
		Priority: prioDBTransactions,
		Dims: module.Dims{
			{ID: "db_%s_total_xact_count", Name: "transactions", Algo: module.Incremental},
		},
	}
	dbTransactionsTimeChartTmpl = module.Chart{
		ID:       "db_%s_transactions_time",
		Title:    "Database transactions time",
		Units:    "seconds",
		Fam:      "transactions time",
		Ctx:      "pgbouncer.db_transactions_time",
		Priority: prioDBTransactionsTime,
		Dims: module.Dims{
			{ID: "db_%s_total_xact_time", Name: "time", Algo: module.Incremental, Div: 1e6},
		},
	}
	dbTransactionAvgTimeChartTmpl = module.Chart{
		ID:       "db_%s_transactions_average_time",
		Title:    "Database transaction average time",
		Units:    "seconds",
		Fam:      "transaction avg time",
		Ctx:      "pgbouncer.db_transaction_avg_time",
		Priority: prioDBTransactionsAvgTime,
		Dims: module.Dims{
			{ID: "db_%s_avg_xact_time", Name: "time", Algo: module.Incremental, Div: 1e6},
		},
	}

	dbQueriesChartTmpl = module.Chart{
		ID:       "db_%s_queries",
		Title:    "Database pooled SQL queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "pgbouncer.db_queries",
		Priority: prioDBQueries,
		Dims: module.Dims{
			{ID: "db_%s_total_query_count", Name: "queries", Algo: module.Incremental},
		},
	}
	dbQueriesTimeChartTmpl = module.Chart{
		ID:       "db_%s_queries_time",
		Title:    "Database queries time",
		Units:    "seconds",
		Fam:      "queries time",
		Ctx:      "pgbouncer.db_queries_time",
		Priority: prioDBQueriesTime,
		Dims: module.Dims{
			{ID: "db_%s_total_query_time", Name: "time", Algo: module.Incremental, Div: 1e6},
		},
	}
	dbQueryAvgTimeChartTmpl = module.Chart{
		ID:       "db_%s_query_average_time",
		Title:    "Database query average time",
		Units:    "seconds",
		Fam:      "query avg time",
		Ctx:      "pgbouncer.db_query_avg_time",
		Priority: prioDBQueryAvgTime,
		Dims: module.Dims{
			{ID: "db_%s_avg_query_time", Name: "time", Algo: module.Incremental, Div: 1e6},
		},
	}

	dbNetworkIOChartTmpl = module.Chart{
		ID:       "db_%s_network_io",
		Title:    "Database traffic",
		Units:    "B/s",
		Fam:      "traffic",
		Ctx:      "pgbouncer.db_network_io",
		Priority: prioDBNetworkIO,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "db_%s_total_received", Name: "received", Algo: module.Incremental},
			{ID: "db_%s_total_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
)

func newDatabaseCharts(dbname, pgDBName string) *module.Charts {
	charts := dbChartsTmpl.Copy()
	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, dbname)
		c.Labels = []module.Label{
			{Key: "database", Value: dbname},
			{Key: "postgres_database", Value: pgDBName},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, dbname)
		}
	}
	return charts
}

func (c *Collector) addNewDatabaseCharts(dbname, pgDBName string) {
	charts := newDatabaseCharts(dbname, pgDBName)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDatabaseCharts(dbname string) {
	prefix := fmt.Sprintf("db_%s_", dbname)
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}
