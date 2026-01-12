// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// TODO: check https://github.com/ProxySQL/proxysql-grafana-prometheus/blob/main/grafana/provisioning/dashboards/ProxySQL-Host-Statistics.json

const (
	prioClientConnectionsCount = module.Priority + iota
	prioClientConnectionsRate
	prioServerConnectionsCount
	prioServerConnectionsRate
	prioBackendsTraffic
	prioFrontendsTraffic
	prioActiveTransactionsCount
	prioQuestionsRate
	prioSlowQueriesRate
	prioQueriesRate
	prioBackendStatementsCount
	prioBackendStatementsRate
	prioFrontendStatementsCount
	prioFrontendStatementsRate
	prioCachedStatementsCount
	prioQueryCacheEntriesCount
	prioQueryCacheIO
	prioQueryCacheRequestsRate
	prioQueryCacheMemoryUsed
	prioMySQLMonitorWorkersCount
	prioMySQLMonitorWorkersRate
	prioMySQLMonitorConnectChecksRate
	prioMySQLMonitorPingChecksRate
	prioMySQLMonitorReadOnlyChecksRate
	prioMySQLMonitorReplicationLagChecksRate
	prioJemallocMemoryUsed
	prioMemoryUsed
	prioMySQLCommandExecutionsRate
	prioMySQLCommandExecutionTime
	prioMySQLCommandExecutionDurationHistogram
	prioMySQLUserConnectionsUtilization
	prioMySQLUserConnectionsCount
	prioHostgroupStatus
	prioBackendStatus
	prioBackendConnectionsUsage
	prioBackendConnectionsRate
	prioBackendQueriesRateRate
	prioBackendTraffic
	prioBackendLatency
	prioUptime
)

var (
	baseCharts = module.Charts{
		clientConnectionsCountChart.Copy(),
		clientConnectionsRateChart.Copy(),
		serverConnectionsCountChart.Copy(),
		serverConnectionsRateChart.Copy(),
		backendsTrafficChart.Copy(),
		frontendsTrafficChart.Copy(),
		activeTransactionsCountChart.Copy(),
		questionsRateChart.Copy(),
		slowQueriesRateChart.Copy(),
		queriesRateChart.Copy(),
		backendStatementsCountChart.Copy(),
		backendStatementsRateChart.Copy(),
		clientStatementsCountChart.Copy(),
		clientStatementsRateChart.Copy(),
		cachedStatementsCountChart.Copy(),
		queryCacheEntriesCountChart.Copy(),
		queryCacheIOChart.Copy(),
		queryCacheRequestsRateChart.Copy(),
		queryCacheMemoryUsedChart.Copy(),
		mySQLMonitorWorkersCountChart.Copy(),
		mySQLMonitorWorkersRateChart.Copy(),
		mySQLMonitorConnectChecksRateChart.Copy(),
		mySQLMonitorPingChecksRateChart.Copy(),
		mySQLMonitorReadOnlyChecksRateChart.Copy(),
		mySQLMonitorReplicationLagChecksRateChart.Copy(),
		jemallocMemoryUsedChart.Copy(),
		memoryUsedCountChart.Copy(),
		uptimeChart.Copy(),
	}

	clientConnectionsCountChart = module.Chart{
		ID:       "client_connections_count",
		Title:    "Client connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "proxysql.client_connections_count",
		Priority: prioClientConnectionsCount,
		Dims: module.Dims{
			{ID: "Client_Connections_connected", Name: "connected"},
			{ID: "Client_Connections_non_idle", Name: "non_idle"},
			{ID: "Client_Connections_hostgroup_locked", Name: "hostgroup_locked"},
		},
	}
	clientConnectionsRateChart = module.Chart{
		ID:       "client_connections_rate",
		Title:    "Client connections rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "proxysql.client_connections_rate",
		Priority: prioClientConnectionsRate,
		Dims: module.Dims{
			{ID: "Client_Connections_created", Name: "created", Algo: module.Incremental},
			{ID: "Client_Connections_aborted", Name: "aborted", Algo: module.Incremental},
		},
	}

	serverConnectionsCountChart = module.Chart{
		ID:       "server_connections_count",
		Title:    "Server connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "proxysql.server_connections_count",
		Priority: prioServerConnectionsCount,
		Dims: module.Dims{
			{ID: "Server_Connections_connected", Name: "connected"},
		},
	}
	serverConnectionsRateChart = module.Chart{
		ID:       "server_connections_rate",
		Title:    "Server connections rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "proxysql.server_connections_rate",
		Priority: prioServerConnectionsRate,
		Dims: module.Dims{
			{ID: "Server_Connections_created", Name: "created", Algo: module.Incremental},
			{ID: "Server_Connections_aborted", Name: "aborted", Algo: module.Incremental},
			{ID: "Server_Connections_delayed", Name: "delayed", Algo: module.Incremental},
		},
	}

	backendsTrafficChart = module.Chart{
		ID:       "backends_traffic",
		Title:    "Backends traffic",
		Units:    "B/s",
		Fam:      "traffic",
		Ctx:      "proxysql.backends_traffic",
		Priority: prioBackendsTraffic,
		Dims: module.Dims{
			{ID: "Queries_backends_bytes_recv", Name: "recv", Algo: module.Incremental},
			{ID: "Queries_backends_bytes_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	frontendsTrafficChart = module.Chart{
		ID:       "clients_traffic",
		Title:    "Clients traffic",
		Units:    "B/s",
		Fam:      "traffic",
		Ctx:      "proxysql.clients_traffic",
		Priority: prioFrontendsTraffic,
		Dims: module.Dims{
			{ID: "Queries_frontends_bytes_recv", Name: "recv", Algo: module.Incremental},
			{ID: "Queries_frontends_bytes_sent", Name: "sent", Algo: module.Incremental},
		},
	}

	activeTransactionsCountChart = module.Chart{
		ID:       "active_transactions_count",
		Title:    "Client connections that are currently processing a transaction",
		Units:    "transactions",
		Fam:      "transactions",
		Ctx:      "proxysql.active_transactions_count",
		Priority: prioActiveTransactionsCount,
		Dims: module.Dims{
			{ID: "Active_Transactions", Name: "active"},
		},
	}
	questionsRateChart = module.Chart{
		ID:       "questions_rate",
		Title:    "Client requests / statements executed",
		Units:    "questions/s",
		Fam:      "queries",
		Ctx:      "proxysql.questions_rate",
		Priority: prioQuestionsRate,
		Dims: module.Dims{
			{ID: "Questions", Name: "questions", Algo: module.Incremental},
		},
	}
	slowQueriesRateChart = module.Chart{
		ID:       "slow_queries_rate",
		Title:    "Slow queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "proxysql.slow_queries_rate",
		Priority: prioSlowQueriesRate,
		Dims: module.Dims{
			{ID: "Slow_queries", Name: "slow", Algo: module.Incremental},
		},
	}
	queriesRateChart = module.Chart{
		ID:       "queries_rate",
		Title:    "Queries rate",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "proxysql.queries_rate",
		Priority: prioQueriesRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "Com_autocommit", Name: "autocommit", Algo: module.Incremental},
			{ID: "Com_autocommit_filtered", Name: "autocommit_filtered", Algo: module.Incremental},
			{ID: "Com_commit", Name: "commit", Algo: module.Incremental},
			{ID: "Com_commit_filtered", Name: "commit_filtered", Algo: module.Incremental},
			{ID: "Com_rollback", Name: "rollback", Algo: module.Incremental},
			{ID: "Com_rollback_filtered", Name: "rollback_filtered", Algo: module.Incremental},
			{ID: "Com_backend_change_user", Name: "backend_change_user", Algo: module.Incremental},
			{ID: "Com_backend_init_db", Name: "backend_init_db", Algo: module.Incremental},
			{ID: "Com_backend_set_names", Name: "backend_set_names", Algo: module.Incremental},
			{ID: "Com_frontend_init_db", Name: "frontend_init_db", Algo: module.Incremental},
			{ID: "Com_frontend_set_names", Name: "frontend_set_names", Algo: module.Incremental},
			{ID: "Com_frontend_use_db", Name: "frontend_use_db", Algo: module.Incremental},
		},
	}

	backendStatementsCountChart = module.Chart{
		ID:       "backend_statements_count",
		Title:    "Statements available across all backend connections",
		Units:    "statements",
		Fam:      "statements",
		Ctx:      "proxysql.backend_statements_count",
		Priority: prioBackendStatementsCount,
		Dims: module.Dims{
			{ID: "Stmt_Server_Active_Total", Name: "total"},
			{ID: "Stmt_Server_Active_Unique", Name: "unique"},
		},
	}
	backendStatementsRateChart = module.Chart{
		ID:       "backend_statements_rate",
		Title:    "Statements executed against the backends",
		Units:    "statements/s",
		Fam:      "statements",
		Ctx:      "proxysql.backend_statements_rate",
		Priority: prioBackendStatementsRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "Com_backend_stmt_prepare", Name: "prepare", Algo: module.Incremental},
			{ID: "Com_backend_stmt_execute", Name: "execute", Algo: module.Incremental},
			{ID: "Com_backend_stmt_close", Name: "close", Algo: module.Incremental},
		},
	}
	clientStatementsCountChart = module.Chart{
		ID:       "client_statements_count",
		Title:    "Statements that are in use by clients",
		Units:    "statements",
		Fam:      "statements",
		Ctx:      "proxysql.client_statements_count",
		Priority: prioFrontendStatementsCount,
		Dims: module.Dims{
			{ID: "Stmt_Client_Active_Total", Name: "total"},
			{ID: "Stmt_Client_Active_Unique", Name: "unique"},
		},
	}
	clientStatementsRateChart = module.Chart{
		ID:       "client_statements_rate",
		Title:    "Statements executed by clients",
		Units:    "statements/s",
		Fam:      "statements",
		Ctx:      "proxysql.client_statements_rate",
		Priority: prioFrontendStatementsRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "Com_frontend_stmt_prepare", Name: "prepare", Algo: module.Incremental},
			{ID: "Com_frontend_stmt_execute", Name: "execute", Algo: module.Incremental},
			{ID: "Com_frontend_stmt_close", Name: "close", Algo: module.Incremental},
		},
	}
	cachedStatementsCountChart = module.Chart{
		ID:       "cached_statements_count",
		Title:    "Global prepared statements",
		Units:    "statements",
		Fam:      "statements",
		Ctx:      "proxysql.cached_statements_count",
		Priority: prioCachedStatementsCount,
		Dims: module.Dims{
			{ID: "Stmt_Cached", Name: "cached"},
		},
	}

	queryCacheEntriesCountChart = module.Chart{
		ID:       "query_cache_entries_count",
		Title:    "Query Cache entries",
		Units:    "entries",
		Fam:      "query cache",
		Ctx:      "proxysql.query_cache_entries_count",
		Priority: prioQueryCacheEntriesCount,
		Dims: module.Dims{
			{ID: "Query_Cache_Entries", Name: "entries"},
		},
	}
	queryCacheMemoryUsedChart = module.Chart{
		ID:       "query_cache_memory_used",
		Title:    "Query Cache memory used",
		Units:    "B",
		Fam:      "query cache",
		Ctx:      "proxysql.query_cache_memory_used",
		Priority: prioQueryCacheMemoryUsed,
		Dims: module.Dims{
			{ID: "Query_Cache_Memory_bytes", Name: "used"},
		},
	}
	queryCacheIOChart = module.Chart{
		ID:       "query_cache_io",
		Title:    "Query Cache I/O",
		Units:    "B/s",
		Fam:      "query cache",
		Ctx:      "proxysql.query_cache_io",
		Priority: prioQueryCacheIO,
		Dims: module.Dims{
			{ID: "Query_Cache_bytes_IN", Name: "in", Algo: module.Incremental},
			{ID: "Query_Cache_bytes_OUT", Name: "out", Algo: module.Incremental},
		},
	}
	queryCacheRequestsRateChart = module.Chart{
		ID:       "query_cache_requests_rate",
		Title:    "Query Cache requests",
		Units:    "requests/s",
		Fam:      "query cache",
		Ctx:      "proxysql.query_cache_requests_rate",
		Priority: prioQueryCacheRequestsRate,
		Dims: module.Dims{
			{ID: "Query_Cache_count_GET", Name: "read", Algo: module.Incremental},
			{ID: "Query_Cache_count_SET", Name: "write", Algo: module.Incremental},
			{ID: "Query_Cache_count_GET_OK", Name: "read_success", Algo: module.Incremental},
		},
	}

	mySQLMonitorWorkersCountChart = module.Chart{
		ID:       "mysql_monitor_workers_count",
		Title:    "MySQL monitor workers",
		Units:    "threads",
		Fam:      "monitor",
		Ctx:      "proxysql.mysql_monitor_workers_count",
		Priority: prioMySQLMonitorWorkersCount,
		Dims: module.Dims{
			{ID: "MySQL_Monitor_Workers", Name: "workers"},
			{ID: "MySQL_Monitor_Workers_Aux", Name: "auxiliary"},
		},
	}
	mySQLMonitorWorkersRateChart = module.Chart{
		ID:       "mysql_monitor_workers_rate",
		Title:    "MySQL monitor workers rate",
		Units:    "workers/s",
		Fam:      "monitor",
		Ctx:      "proxysql.mysql_monitor_workers_rate",
		Priority: prioMySQLMonitorWorkersRate,
		Dims: module.Dims{
			{ID: "MySQL_Monitor_Workers_Started", Name: "started", Algo: module.Incremental},
		},
	}
	mySQLMonitorConnectChecksRateChart = module.Chart{
		ID:       "mysql_monitor_connect_checks_rate",
		Title:    "MySQL monitor connect checks",
		Units:    "checks/s",
		Fam:      "monitor",
		Ctx:      "proxysql.mysql_monitor_connect_checks_rate",
		Priority: prioMySQLMonitorConnectChecksRate,
		Dims: module.Dims{
			{ID: "MySQL_Monitor_connect_check_OK", Name: "succeed", Algo: module.Incremental},
			{ID: "MySQL_Monitor_connect_check_ERR", Name: "failed", Algo: module.Incremental},
		},
	}
	mySQLMonitorPingChecksRateChart = module.Chart{
		ID:       "mysql_monitor_ping_checks_rate",
		Title:    "MySQL monitor ping checks",
		Units:    "checks/s",
		Fam:      "monitor",
		Ctx:      "proxysql.mysql_monitor_ping_checks_rate",
		Priority: prioMySQLMonitorPingChecksRate,
		Dims: module.Dims{
			{ID: "MySQL_Monitor_ping_check_OK", Name: "succeed", Algo: module.Incremental},
			{ID: "MySQL_Monitor_ping_check_ERR", Name: "failed", Algo: module.Incremental},
		},
	}
	mySQLMonitorReadOnlyChecksRateChart = module.Chart{
		ID:       "mysql_monitor_read_only_checks_rate",
		Title:    "MySQL monitor read only checks",
		Units:    "checks/s",
		Fam:      "monitor",
		Ctx:      "proxysql.mysql_monitor_read_only_checks_rate",
		Priority: prioMySQLMonitorReadOnlyChecksRate,
		Dims: module.Dims{
			{ID: "MySQL_Monitor_read_only_check_OK", Name: "succeed", Algo: module.Incremental},
			{ID: "MySQL_Monitor_read_only_check_ERR", Name: "failed", Algo: module.Incremental},
		},
	}
	mySQLMonitorReplicationLagChecksRateChart = module.Chart{
		ID:       "mysql_monitor_replication_lag_checks_rate",
		Title:    "MySQL monitor replication lag checks",
		Units:    "checks/s",
		Fam:      "monitor",
		Ctx:      "proxysql.mysql_monitor_replication_lag_checks_rate",
		Priority: prioMySQLMonitorReplicationLagChecksRate,
		Dims: module.Dims{
			{ID: "MySQL_Monitor_replication_lag_check_OK", Name: "succeed", Algo: module.Incremental},
			{ID: "MySQL_Monitor_replication_lag_check_ERR", Name: "failed", Algo: module.Incremental},
		},
	}

	jemallocMemoryUsedChart = module.Chart{
		ID:       "jemalloc_memory_used",
		Title:    "Jemalloc used memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "proxysql.jemalloc_memory_used",
		Type:     module.Stacked,
		Priority: prioJemallocMemoryUsed,
		Dims: module.Dims{
			{ID: "jemalloc_active", Name: "active"},
			{ID: "jemalloc_allocated", Name: "allocated"},
			{ID: "jemalloc_mapped", Name: "mapped"},
			{ID: "jemalloc_metadata", Name: "metadata"},
			{ID: "jemalloc_resident", Name: "resident"},
			{ID: "jemalloc_retained", Name: "retained"},
		},
	}
	memoryUsedCountChart = module.Chart{
		ID:       "memory_used",
		Title:    "Memory used",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "proxysql.memory_used",
		Priority: prioMemoryUsed,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "Auth_memory", Name: "auth"},
			{ID: "SQLite3_memory_bytes", Name: "sqlite3"},
			{ID: "query_digest_memory", Name: "query_digest"},
			{ID: "mysql_query_rules_memory", Name: "query_rules"},
			{ID: "mysql_firewall_users_table", Name: "firewall_users_table"},
			{ID: "mysql_firewall_users_config", Name: "firewall_users_config"},
			{ID: "mysql_firewall_rules_table", Name: "firewall_rules_table"},
			{ID: "mysql_firewall_rules_config", Name: "firewall_rules_config"},
			{ID: "stack_memory_mysql_threads", Name: "mysql_threads"},
			{ID: "stack_memory_admin_threads", Name: "admin_threads"},
			{ID: "stack_memory_cluster_threads", Name: "cluster_threads"},
		},
	}
	uptimeChart = module.Chart{
		ID:       "proxysql_uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "proxysql.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "ProxySQL_Uptime", Name: "uptime"},
		},
	}
)

var (
	mySQLCommandChartsTmpl = module.Charts{
		mySQLCommandExecutionRateChartTmpl.Copy(),
		mySQLCommandExecutionTimeChartTmpl.Copy(),
		mySQLCommandExecutionDurationHistogramChartTmpl.Copy(),
	}

	mySQLCommandExecutionRateChartTmpl = module.Chart{
		ID:       "mysql_command_%s_execution_rate",
		Title:    "MySQL command execution",
		Units:    "commands/s",
		Fam:      "command exec",
		Ctx:      "proxysql.mysql_command_execution_rate",
		Priority: prioMySQLCommandExecutionsRate,
		Dims: module.Dims{
			{ID: "mysql_command_%s_Total_cnt", Name: "commands", Algo: module.Incremental},
		},
	}
	mySQLCommandExecutionTimeChartTmpl = module.Chart{
		ID:       "mysql_command_%s_execution_time",
		Title:    "MySQL command execution time",
		Units:    "microseconds",
		Fam:      "command exec time",
		Ctx:      "proxysql.mysql_command_execution_time",
		Priority: prioMySQLCommandExecutionTime,
		Dims: module.Dims{
			{ID: "mysql_command_%s_Total_Time_us", Name: "time", Algo: module.Incremental},
		},
	}
	mySQLCommandExecutionDurationHistogramChartTmpl = module.Chart{
		ID:       "mysql_command_%s_execution_duration",
		Title:    "MySQL command execution duration histogram",
		Units:    "commands/s",
		Fam:      "command exec duration",
		Ctx:      "proxysql.mysql_command_execution_duration",
		Type:     module.Stacked,
		Priority: prioMySQLCommandExecutionDurationHistogram,
		Dims: module.Dims{
			{ID: "mysql_command_%s_cnt_100us", Name: "100us", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_500us", Name: "500us", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_1ms", Name: "1ms", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_5ms", Name: "5ms", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_10ms", Name: "10ms", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_50ms", Name: "50ms", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_100ms", Name: "100ms", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_500ms", Name: "500ms", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_1s", Name: "1s", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_5s", Name: "5s", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_10s", Name: "10s", Algo: module.Incremental},
			{ID: "mysql_command_%s_cnt_INFs", Name: "+Inf", Algo: module.Incremental},
		},
	}
)

func newMySQLCommandCountersCharts(command string) *module.Charts {
	charts := mySQLCommandChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(command))
		chart.Labels = []module.Label{{Key: "command", Value: command}}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, command)
		}
	}

	return charts
}

func (c *Collector) addMySQLCommandCountersCharts(command string) {
	charts := newMySQLCommandCountersCharts(command)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeMySQLCommandCountersCharts(command string) {
	prefix := "mysql_command_" + strings.ToLower(command)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

var (
	mySQLUserChartsTmpl = module.Charts{
		mySQLUserConnectionsUtilizationChartTmpl.Copy(),
		mySQLUserConnectionsCountChartTmpl.Copy(),
	}

	mySQLUserConnectionsUtilizationChartTmpl = module.Chart{
		ID:       "mysql_user_%s_connections_utilization",
		Title:    "MySQL user connections utilization",
		Units:    "percentage",
		Fam:      "user conns",
		Ctx:      "proxysql.mysql_user_connections_utilization",
		Priority: prioMySQLUserConnectionsUtilization,
		Dims: module.Dims{
			{ID: "mysql_user_%s_frontend_connections_utilization", Name: "used"},
		},
	}
	mySQLUserConnectionsCountChartTmpl = module.Chart{
		ID:       "mysql_user_%s_connections_count",
		Title:    "MySQL user connections used",
		Units:    "connections",
		Fam:      "user conns",
		Ctx:      "proxysql.mysql_user_connections_count",
		Priority: prioMySQLUserConnectionsCount,
		Dims: module.Dims{
			{ID: "mysql_user_%s_frontend_connections", Name: "used"},
		},
	}
)

func newMySQLUserCharts(username string) *module.Charts {
	charts := mySQLUserChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, username)
		chart.Labels = []module.Label{{Key: "user", Value: username}}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, username)
		}
	}

	return charts
}

func (c *Collector) addMySQLUsersCharts(username string) {
	charts := newMySQLUserCharts(username)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeMySQLUserCharts(user string) {
	prefix := "mysql_user_" + user

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

var (
	hostgroupChartsTmpl = module.Charts{
		hostgroupBackendsStatusChartTmpl.Copy(),
	}

	hostgroupBackendsStatusChartTmpl = module.Chart{
		ID:       "hostgroup_%s_backends_status",
		Title:    "Hostgroup backends count in each status",
		Units:    "backends",
		Fam:      "hostgroup backends",
		Ctx:      "proxysql.hostgroup_backends_status",
		Priority: prioHostgroupStatus,
		Dims: module.Dims{
			{ID: "hostgroup_%s_backends_ONLINE", Name: "online"},
			{ID: "hostgroup_%s_backends_SHUNNED", Name: "shunned"},
			{ID: "hostgroup_%s_backends_OFFLINE_SOFT", Name: "offline_soft"},
			{ID: "hostgroup_%s_backends_OFFLINE_HARD", Name: "offline_hard"},
		},
	}
)

func newHostgroupCharts(hg string) *module.Charts {
	charts := hostgroupChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, hg)
		chart.Labels = []module.Label{
			{Key: "hostgroup", Value: hg},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, hg)
		}
	}

	return charts
}

func (c *Collector) addHostgroupCharts(hg string) {
	charts := newHostgroupCharts(hg)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHostgroupCharts(hg string) {
	prefix := "hostgroup_" + hg

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

var (
	backendChartsTmpl = module.Charts{
		backendStatusChartTmpl.Copy(),
		backendConnectionsUsageChartTmpl.Copy(),
		backendConnectionsRateChartTmpl.Copy(),
		backendQueriesRateRateChartTmpl.Copy(),
		backendTrafficChartTmpl.Copy(),
		backendLatencyChartTmpl.Copy(),
	}

	backendStatusChartTmpl = module.Chart{
		ID:       "backend_%s_status",
		Title:    "Backend status",
		Units:    "status",
		Fam:      "backend status",
		Ctx:      "proxysql.backend_status",
		Priority: prioBackendStatus,
		Dims: module.Dims{
			{ID: "backend_%s_status_ONLINE", Name: "online"},
			{ID: "backend_%s_status_SHUNNED", Name: "shunned"},
			{ID: "backend_%s_status_OFFLINE_SOFT", Name: "offline_soft"},
			{ID: "backend_%s_status_OFFLINE_HARD", Name: "offline_hard"},
		},
	}
	backendConnectionsUsageChartTmpl = module.Chart{
		ID:       "backend_%s_connections_usage",
		Title:    "Backend connections usage",
		Units:    "connections",
		Fam:      "backend conns usage",
		Ctx:      "proxysql.backend_connections_usage",
		Type:     module.Stacked,
		Priority: prioBackendConnectionsUsage,
		Dims: module.Dims{
			{ID: "backend_%s_ConnFree", Name: "free"},
			{ID: "backend_%s_ConnUsed", Name: "used"},
		},
	}
	backendConnectionsRateChartTmpl = module.Chart{
		ID:       "backend_%s_connections_rate",
		Title:    "Backend connections established",
		Units:    "connections/s",
		Fam:      "backend conns established",
		Ctx:      "proxysql.backend_connections_rate",
		Priority: prioBackendConnectionsRate,
		Dims: module.Dims{
			{ID: "backend_%s_ConnOK", Name: "succeed", Algo: module.Incremental},
			{ID: "backend_%s_ConnERR", Name: "failed", Algo: module.Incremental},
		},
	}
	backendQueriesRateRateChartTmpl = module.Chart{
		ID:       "backend_%s_queries_rate",
		Title:    "Backend queries",
		Units:    "queries/s",
		Fam:      "backend queries",
		Ctx:      "proxysql.backend_queries_rate",
		Priority: prioBackendQueriesRateRate,
		Dims: module.Dims{
			{ID: "backend_%s_Queries", Name: "queries", Algo: module.Incremental},
		},
	}
	backendTrafficChartTmpl = module.Chart{
		ID:       "backend_%s_traffic",
		Title:    "Backend traffic",
		Units:    "B/s",
		Fam:      "backend traffic",
		Ctx:      "proxysql.backend_traffic",
		Priority: prioBackendTraffic,
		Dims: module.Dims{
			{ID: "backend_%s_Bytes_data_recv", Name: "recv", Algo: module.Incremental},
			{ID: "backend_%s_Bytes_data_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	backendLatencyChartTmpl = module.Chart{
		ID:       "backend_%s_latency",
		Title:    "Backend latency",
		Units:    "microseconds",
		Fam:      "backend latency",
		Ctx:      "proxysql.backend_latency",
		Priority: prioBackendLatency,
		Dims: module.Dims{
			{ID: "backend_%s_Latency_us", Name: "latency"},
		},
	}
)

func newBackendCharts(hg, host, port string) *module.Charts {
	charts := backendChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, backendID(hg, host, port))
		chart.Labels = []module.Label{
			{Key: "hostgroup", Value: hg},
			{Key: "host", Value: host},
			{Key: "port", Value: port},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, backendID(hg, host, port))
		}
	}

	return charts
}

func (c *Collector) addBackendCharts(hg, host, port string) {
	charts := newBackendCharts(hg, host, port)

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeBackendCharts(hg, host, port string) {
	prefix := "backend_" + backendID(hg, host, port)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
