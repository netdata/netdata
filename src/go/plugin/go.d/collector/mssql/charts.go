// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioUserConnections = module.Priority + iota
	prioSessionConnections
	prioBlockedProcesses

	prioBatchRequests
	prioCompilations
	prioRecompilations
	prioAutoParamAttempts
	prioSQLErrors

	prioBufferCacheHitRatio
	prioBufferPageIOPS
	prioBufferCheckpointPages
	prioBufferPageLifeExpectancy
	prioBufferLazyWrites
	prioBufferPageLookups

	prioPageSplits

	prioMemoryTotal
	prioMemoryConnection
	prioMemoryPendingGrants
	prioMemoryExternalBenefit

	prioDatabaseActiveTransactions
	prioDatabaseTransactions
	prioDatabaseWriteTransactions
	prioDatabaseLogFlushes
	prioDatabaseLogFlushed
	prioDatabaseDeadlocks
	prioDatabaseLockWaits
	prioDatabaseLockTimeouts
	prioDatabaseLockRequests
	prioDatabaseDataFileSize
	prioDatabaseState
	prioDatabaseReadOnly

	prioLocksByResource

	prioWaitTotalTime
	prioWaitResourceTime
	prioWaitSignalTime
	prioWaitMaxTime
	prioWaitCount

	prioJobStatus

	prioReplicationStatus
	prioReplicationLatency
)

// instanceCharts are charts for the SQL Server instance level metrics
var instanceCharts = module.Charts{
	userConnectionsChart.Copy(),
	sessionConnectionsChart.Copy(),
	blockedProcessesChart.Copy(),

	batchRequestsChart.Copy(),
	compilationsChart.Copy(),
	recompilationsChart.Copy(),
	autoParamAttemptsChart.Copy(),
	sqlErrorsChart.Copy(),

	bufferCacheHitRatioChart.Copy(),
	bufferPageIOPSChart.Copy(),
	bufferCheckpointPagesChart.Copy(),
	bufferPageLifeExpectancyChart.Copy(),
	bufferLazyWritesChart.Copy(),
	bufferPageLookupsChart.Copy(),

	pageSplitsChart.Copy(),

	memoryTotalChart.Copy(),
	memoryConnectionChart.Copy(),
	memoryPendingGrantsChart.Copy(),
	memoryExternalBenefitChart.Copy(),
}

var (
	userConnectionsChart = module.Chart{
		ID:       "user_connections",
		Title:    "User connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mssql.user_connections",
		Priority: prioUserConnections,
		Dims: module.Dims{
			{ID: "user_connections", Name: "user"},
		},
	}
	blockedProcessesChart = module.Chart{
		ID:       "blocked_processes",
		Title:    "Blocked processes",
		Units:    "processes",
		Fam:      "processes",
		Ctx:      "mssql.blocked_processes",
		Priority: prioBlockedProcesses,
		Dims: module.Dims{
			{ID: "blocked_processes", Name: "blocked"},
		},
	}
	sessionConnectionsChart = module.Chart{
		ID:       "session_connections",
		Title:    "Session connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mssql.session_connections",
		Priority: prioSessionConnections,
		Dims: module.Dims{
			{ID: "session_connections_user", Name: "user"},
			{ID: "session_connections_internal", Name: "internal"},
		},
	}

	batchRequestsChart = module.Chart{
		ID:       "batch_requests",
		Title:    "Batch requests",
		Units:    "requests/s",
		Fam:      "queries",
		Ctx:      "mssql.batch_requests",
		Priority: prioBatchRequests,
		Dims: module.Dims{
			{ID: "batch_requests", Name: "batch", Algo: module.Incremental},
		},
	}
	compilationsChart = module.Chart{
		ID:       "compilations",
		Title:    "SQL compilations",
		Units:    "compilations/s",
		Fam:      "queries",
		Ctx:      "mssql.compilations",
		Priority: prioCompilations,
		Dims: module.Dims{
			{ID: "sql_compilations", Name: "compilations", Algo: module.Incremental},
		},
	}
	recompilationsChart = module.Chart{
		ID:       "recompilations",
		Title:    "SQL re-compilations",
		Units:    "recompilations/s",
		Fam:      "queries",
		Ctx:      "mssql.recompilations",
		Priority: prioRecompilations,
		Dims: module.Dims{
			{ID: "sql_recompilations", Name: "recompilations", Algo: module.Incremental},
		},
	}
	autoParamAttemptsChart = module.Chart{
		ID:       "auto_param_attempts",
		Title:    "Auto-parameterization attempts",
		Units:    "attempts/s",
		Fam:      "queries",
		Ctx:      "mssql.auto_param_attempts",
		Priority: prioAutoParamAttempts,
		Dims: module.Dims{
			{ID: "auto_param_attempts", Name: "total", Algo: module.Incremental},
			{ID: "auto_param_safe", Name: "safe", Algo: module.Incremental},
			{ID: "auto_param_failed", Name: "failed", Algo: module.Incremental, Mul: -1},
		},
	}
	sqlErrorsChart = module.Chart{
		ID:       "sql_errors",
		Title:    "SQL errors",
		Units:    "errors/s",
		Fam:      "errors",
		Ctx:      "mssql.sql_errors",
		Priority: prioSQLErrors,
		Dims: module.Dims{
			{ID: "sql_errors_total", Name: "errors", Algo: module.Incremental},
		},
	}

	bufferCacheHitRatioChart = module.Chart{
		ID:       "buffer_cache_hit_ratio",
		Title:    "Buffer cache hit ratio",
		Units:    "percentage",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_cache_hit_ratio",
		Priority: prioBufferCacheHitRatio,
		Dims: module.Dims{
			{ID: "buffer_cache_hit_ratio", Name: "hit_ratio"},
		},
	}
	bufferPageIOPSChart = module.Chart{
		ID:       "buffer_page_iops",
		Title:    "Buffer page I/O",
		Units:    "pages/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_page_iops",
		Priority: prioBufferPageIOPS,
		Dims: module.Dims{
			{ID: "buffer_page_reads", Name: "read", Algo: module.Incremental},
			{ID: "buffer_page_writes", Name: "written", Algo: module.Incremental, Mul: -1},
		},
	}
	bufferCheckpointPagesChart = module.Chart{
		ID:       "buffer_checkpoint_pages",
		Title:    "Buffer checkpoint pages flushed",
		Units:    "pages/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_checkpoint_pages",
		Priority: prioBufferCheckpointPages,
		Dims: module.Dims{
			{ID: "buffer_checkpoint_pages", Name: "flushed", Algo: module.Incremental},
		},
	}
	bufferPageLifeExpectancyChart = module.Chart{
		ID:       "buffer_page_life_expectancy",
		Title:    "Buffer page life expectancy",
		Units:    "seconds",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_page_life_expectancy",
		Priority: prioBufferPageLifeExpectancy,
		Dims: module.Dims{
			{ID: "buffer_page_life_expectancy", Name: "life_expectancy"},
		},
	}
	bufferLazyWritesChart = module.Chart{
		ID:       "buffer_lazy_writes",
		Title:    "Buffer lazy writes",
		Units:    "writes/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_lazy_writes",
		Priority: prioBufferLazyWrites,
		Dims: module.Dims{
			{ID: "buffer_lazy_writes", Name: "lazy_writes", Algo: module.Incremental},
		},
	}
	bufferPageLookupsChart = module.Chart{
		ID:       "buffer_page_lookups",
		Title:    "Buffer page lookups",
		Units:    "lookups/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_page_lookups",
		Priority: prioBufferPageLookups,
		Dims: module.Dims{
			{ID: "buffer_page_lookups", Name: "lookups", Algo: module.Incremental},
		},
	}

	pageSplitsChart = module.Chart{
		ID:       "page_splits",
		Title:    "Page splits",
		Units:    "splits/s",
		Fam:      "access",
		Ctx:      "mssql.page_splits",
		Priority: prioPageSplits,
		Dims: module.Dims{
			{ID: "page_splits", Name: "page", Algo: module.Incremental},
		},
	}

	memoryTotalChart = module.Chart{
		ID:       "memory_total",
		Title:    "Total server memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mssql.memory_total",
		Priority: prioMemoryTotal,
		Dims: module.Dims{
			{ID: "memory_total", Name: "memory"},
		},
	}
	memoryConnectionChart = module.Chart{
		ID:       "memory_connection",
		Title:    "Connection memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mssql.memory_connection",
		Priority: prioMemoryConnection,
		Dims: module.Dims{
			{ID: "memory_connection", Name: "memory"},
		},
	}
	memoryPendingGrantsChart = module.Chart{
		ID:       "memory_pending_grants",
		Title:    "Pending memory grants",
		Units:    "processes",
		Fam:      "memory",
		Ctx:      "mssql.memory_pending_grants",
		Priority: prioMemoryPendingGrants,
		Dims: module.Dims{
			{ID: "memory_pending_grants", Name: "pending"},
		},
	}
	memoryExternalBenefitChart = module.Chart{
		ID:       "memory_external_benefit",
		Title:    "External benefit of memory",
		Units:    "benefit",
		Fam:      "memory",
		Ctx:      "mssql.memory_external_benefit",
		Priority: prioMemoryExternalBenefit,
		Dims: module.Dims{
			{ID: "memory_external_benefit", Name: "benefit"},
		},
	}
)

// Database chart templates
var (
	databaseActiveTransactionsChartTmpl = module.Chart{
		ID:       "database_%s_active_transactions",
		Title:    "Active transactions",
		Units:    "transactions",
		Fam:      "db transactions",
		Ctx:      "mssql.database_active_transactions",
		Priority: prioDatabaseActiveTransactions,
		Dims: module.Dims{
			{ID: "database_%s_active_transactions", Name: "active"},
		},
	}
	databaseTransactionsChartTmpl = module.Chart{
		ID:       "database_%s_transactions",
		Title:    "Transactions",
		Units:    "transactions/s",
		Fam:      "db transactions",
		Ctx:      "mssql.database_transactions",
		Priority: prioDatabaseTransactions,
		Dims: module.Dims{
			{ID: "database_%s_transactions", Name: "transactions", Algo: module.Incremental},
		},
	}
	databaseWriteTransactionsChartTmpl = module.Chart{
		ID:       "database_%s_write_transactions",
		Title:    "Write transactions",
		Units:    "transactions/s",
		Fam:      "db transactions",
		Ctx:      "mssql.database_write_transactions",
		Priority: prioDatabaseWriteTransactions,
		Dims: module.Dims{
			{ID: "database_%s_write_transactions", Name: "write", Algo: module.Incremental},
		},
	}
	databaseLogFlushesChartTmpl = module.Chart{
		ID:       "database_%s_log_flushes",
		Title:    "Log flushes",
		Units:    "flushes/s",
		Fam:      "db log",
		Ctx:      "mssql.database_log_flushes",
		Priority: prioDatabaseLogFlushes,
		Dims: module.Dims{
			{ID: "database_%s_log_flushes", Name: "flushes", Algo: module.Incremental},
		},
	}
	databaseLogFlushedChartTmpl = module.Chart{
		ID:       "database_%s_log_flushed",
		Title:    "Log bytes flushed",
		Units:    "bytes/s",
		Fam:      "db log",
		Ctx:      "mssql.database_log_flushed",
		Priority: prioDatabaseLogFlushed,
		Dims: module.Dims{
			{ID: "database_%s_log_flushed", Name: "flushed", Algo: module.Incremental},
		},
	}
	databaseDataFileSizeChartTmpl = module.Chart{
		ID:       "database_%s_data_file_size",
		Title:    "Data file size",
		Units:    "bytes",
		Fam:      "db size",
		Ctx:      "mssql.database_data_file_size",
		Priority: prioDatabaseDataFileSize,
		Dims: module.Dims{
			{ID: "database_%s_data_file_size", Name: "size"},
		},
	}
	databaseStateChartTmpl = module.Chart{
		ID:       "database_%s_state",
		Title:    "Database state",
		Units:    "state",
		Fam:      "db status",
		Ctx:      "mssql.database_state",
		Priority: prioDatabaseState,
		Dims: module.Dims{
			{ID: "database_%s_state_online", Name: "online"},
			{ID: "database_%s_state_restoring", Name: "restoring"},
			{ID: "database_%s_state_recovering", Name: "recovering"},
			{ID: "database_%s_state_pending", Name: "pending"},
			{ID: "database_%s_state_suspect", Name: "suspect"},
			{ID: "database_%s_state_emergency", Name: "emergency"},
			{ID: "database_%s_state_offline", Name: "offline"},
		},
	}
	databaseReadOnlyChartTmpl = module.Chart{
		ID:       "database_%s_read_only",
		Title:    "Database read-only status",
		Units:    "status",
		Fam:      "db status",
		Ctx:      "mssql.database_read_only",
		Priority: prioDatabaseReadOnly,
		Dims: module.Dims{
			{ID: "database_%s_read_only", Name: "read_only"},
			{ID: "database_%s_read_write", Name: "read_write"},
		},
	}
)

// Lock stats chart templates (per lock resource type, from performance counters)
var (
	lockStatsDeadlocksChartTmpl = module.Chart{
		ID:       "lock_stats_%s_deadlocks",
		Title:    "Deadlocks by lock resource type",
		Units:    "deadlocks/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_deadlocks",
		Priority: prioDatabaseDeadlocks,
		Dims: module.Dims{
			{ID: "lock_stats_%s_deadlocks", Name: "deadlocks", Algo: module.Incremental},
		},
	}
	lockStatsWaitsChartTmpl = module.Chart{
		ID:       "lock_stats_%s_waits",
		Title:    "Lock waits by lock resource type",
		Units:    "waits/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_waits",
		Priority: prioDatabaseLockWaits,
		Dims: module.Dims{
			{ID: "lock_stats_%s_waits", Name: "waits", Algo: module.Incremental},
		},
	}
	lockStatsTimeoutsChartTmpl = module.Chart{
		ID:       "lock_stats_%s_timeouts",
		Title:    "Lock timeouts by lock resource type",
		Units:    "timeouts/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_timeouts",
		Priority: prioDatabaseLockTimeouts,
		Dims: module.Dims{
			{ID: "lock_stats_%s_timeouts", Name: "timeouts", Algo: module.Incremental},
		},
	}
	lockStatsRequestsChartTmpl = module.Chart{
		ID:       "lock_stats_%s_requests",
		Title:    "Lock requests by lock resource type",
		Units:    "requests/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_requests",
		Priority: prioDatabaseLockRequests,
		Dims: module.Dims{
			{ID: "lock_stats_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	}
)

// Lock resource chart template
var locksByResourceChartTmpl = module.Chart{
	ID:       "locks_by_resource_%s",
	Title:    "Locks by resource type",
	Units:    "locks",
	Fam:      "locks",
	Ctx:      "mssql.locks_by_resource",
	Priority: prioLocksByResource,
	Dims: module.Dims{
		{ID: "locks_%s_count", Name: "locks"},
	},
}

// Wait type chart templates
var (
	waitTotalTimeChartTmpl = module.Chart{
		ID:       "wait_%s_total_time",
		Title:    "Total wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_total_time",
		Priority: prioWaitTotalTime,
		Dims: module.Dims{
			{ID: "wait_%s_total_ms", Name: "duration", Algo: module.Incremental},
		},
	}
	waitResourceTimeChartTmpl = module.Chart{
		ID:       "wait_%s_resource_time",
		Title:    "Resource wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_resource_time",
		Priority: prioWaitResourceTime,
		Dims: module.Dims{
			{ID: "wait_%s_resource_ms", Name: "duration", Algo: module.Incremental},
		},
	}
	waitSignalTimeChartTmpl = module.Chart{
		ID:       "wait_%s_signal_time",
		Title:    "Signal wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_signal_time",
		Priority: prioWaitSignalTime,
		Dims: module.Dims{
			{ID: "wait_%s_signal_ms", Name: "duration", Algo: module.Incremental},
		},
	}
	waitCountChartTmpl = module.Chart{
		ID:       "wait_%s_count",
		Title:    "Wait count",
		Units:    "waits/s",
		Fam:      "waits",
		Ctx:      "mssql.wait_count",
		Priority: prioWaitCount,
		Dims: module.Dims{
			{ID: "wait_%s_tasks", Name: "waits", Algo: module.Incremental},
		},
	}
)

// Job status chart template
var jobStatusChartTmpl = module.Chart{
	ID:       "job_%s_status",
	Title:    "Job status",
	Units:    "status",
	Fam:      "jobs",
	Ctx:      "mssql.job_status",
	Priority: prioJobStatus,
	Dims: module.Dims{
		{ID: "job_%s_enabled", Name: "enabled"},
		{ID: "job_%s_disabled", Name: "disabled"},
	},
}

// Replication chart templates
var (
	replicationStatusChartTmpl = module.Chart{
		ID:       "replication_%s_status",
		Title:    "Replication status",
		Units:    "status",
		Fam:      "replication",
		Ctx:      "mssql.replication_status",
		Priority: prioReplicationStatus,
		Dims: module.Dims{
			{ID: "replication_%s_status", Name: "status"},
			{ID: "replication_%s_warning", Name: "warning"},
		},
	}
	replicationLatencyChartTmpl = module.Chart{
		ID:       "replication_%s_latency",
		Title:    "Replication latency",
		Units:    "seconds",
		Fam:      "replication",
		Ctx:      "mssql.replication_latency",
		Priority: prioReplicationLatency,
		Dims: module.Dims{
			{ID: "replication_%s_latency_avg", Name: "average"},
			{ID: "replication_%s_latency_best", Name: "best"},
			{ID: "replication_%s_latency_worst", Name: "worst"},
		},
	}
	replicationSubscriptionsChartTmpl = module.Chart{
		ID:       "replication_%s_subscriptions",
		Title:    "Replication subscriptions",
		Units:    "subscriptions",
		Fam:      "replication",
		Ctx:      "mssql.replication_subscriptions",
		Priority: prioReplicationStatus + 1,
		Dims: module.Dims{
			{ID: "replication_%s_subscriptions", Name: "total"},
			{ID: "replication_%s_agents_running", Name: "agents_running"},
		},
	}
)

func (c *Collector) addDatabaseCharts(dbName string) {
	charts := &module.Charts{
		databaseActiveTransactionsChartTmpl.Copy(),
		databaseTransactionsChartTmpl.Copy(),
		databaseWriteTransactionsChartTmpl.Copy(),
		databaseLogFlushesChartTmpl.Copy(),
		databaseLogFlushedChartTmpl.Copy(),
		databaseDataFileSizeChartTmpl.Copy(),
		databaseStateChartTmpl.Copy(),
		databaseReadOnlyChartTmpl.Copy(),
	}

	dbID := cleanDatabaseName(dbName)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dbID)
		chart.Labels = []module.Label{
			{Key: "database", Value: dbName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dbID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addWaitTypeCharts(waitType string, waitCategory string) {
	charts := &module.Charts{
		waitTotalTimeChartTmpl.Copy(),
		waitResourceTimeChartTmpl.Copy(),
		waitSignalTimeChartTmpl.Copy(),
		waitCountChartTmpl.Copy(),
	}

	waitID := cleanWaitTypeName(waitType)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, waitID)
		chart.Labels = []module.Label{
			{Key: "wait_type", Value: waitType},
			{Key: "wait_category", Value: waitCategory},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, waitID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addLockResourceCharts(resourceType string) {
	chart := locksByResourceChartTmpl.Copy()

	resID := cleanResourceTypeName(resourceType)

	chart.ID = fmt.Sprintf(chart.ID, resID)
	chart.Labels = []module.Label{
		{Key: "resource", Value: resourceType},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, resID)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addLockStatsCharts(resourceType string) {
	charts := &module.Charts{
		lockStatsDeadlocksChartTmpl.Copy(),
		lockStatsWaitsChartTmpl.Copy(),
		lockStatsTimeoutsChartTmpl.Copy(),
		lockStatsRequestsChartTmpl.Copy(),
	}

	resID := cleanResourceTypeName(resourceType)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, resID)
		chart.Labels = []module.Label{
			{Key: "resource", Value: resourceType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, resID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addJobCharts(jobName string) {
	chart := jobStatusChartTmpl.Copy()

	jobID := cleanJobName(jobName)

	chart.ID = fmt.Sprintf(chart.ID, jobID)
	chart.Labels = []module.Label{
		{Key: "job_name", Value: jobName},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, jobID)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addReplicationCharts(pubDB, publication string) {
	charts := &module.Charts{
		replicationStatusChartTmpl.Copy(),
		replicationLatencyChartTmpl.Copy(),
		replicationSubscriptionsChartTmpl.Copy(),
	}

	pubID := cleanPublicationName(pubDB, publication)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pubID)
		chart.Labels = []module.Label{
			{Key: "publisher_db", Value: pubDB},
			{Key: "publication", Value: publication},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, pubID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func cleanDatabaseName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

func cleanWaitTypeName(name string) string {
	return strings.ToLower(name)
}

func cleanResourceTypeName(name string) string {
	return strings.ToLower(name)
}

func cleanJobName(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", "-", "_")
	return strings.ToLower(r.Replace(name))
}

func cleanPublicationName(pubDB, publication string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", "-", "_")
	return strings.ToLower(r.Replace(pubDB + "_" + publication))
}
