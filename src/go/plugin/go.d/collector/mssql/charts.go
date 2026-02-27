// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioUserConnections = collectorapi.Priority + iota
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

	prioProcessMemoryResident
	prioProcessMemoryVirtual
	prioProcessMemoryUtilization
	prioProcessPageFaults

	prioOSMemory
	prioOSPageFile

	prioDatabaseActiveTransactions
	prioDatabaseTransactions
	prioDatabaseWriteTransactions
	prioDatabaseLogFlushes
	prioDatabaseLogFlushed
	prioDatabaseLogGrowths
	prioDatabaseIOStall
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
var instanceCharts = collectorapi.Charts{
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

	processMemoryResidentChart.Copy(),
	processMemoryVirtualChart.Copy(),
	processMemoryUtilizationChart.Copy(),
	processPageFaultsChart.Copy(),

	osMemoryChart.Copy(),
	osPageFileChart.Copy(),
}

var (
	userConnectionsChart = collectorapi.Chart{
		ID:       "user_connections",
		Title:    "User connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mssql.user_connections",
		Priority: prioUserConnections,
		Dims: collectorapi.Dims{
			{ID: "user_connections", Name: "user"},
		},
	}
	blockedProcessesChart = collectorapi.Chart{
		ID:       "blocked_processes",
		Title:    "Blocked processes",
		Units:    "processes",
		Fam:      "processes",
		Ctx:      "mssql.blocked_processes",
		Priority: prioBlockedProcesses,
		Dims: collectorapi.Dims{
			{ID: "blocked_processes", Name: "blocked"},
		},
	}
	sessionConnectionsChart = collectorapi.Chart{
		ID:       "session_connections",
		Title:    "Session connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mssql.session_connections",
		Priority: prioSessionConnections,
		Dims: collectorapi.Dims{
			{ID: "session_connections_user", Name: "user"},
			{ID: "session_connections_internal", Name: "internal"},
		},
	}

	batchRequestsChart = collectorapi.Chart{
		ID:       "batch_requests",
		Title:    "Batch requests",
		Units:    "requests/s",
		Fam:      "queries",
		Ctx:      "mssql.batch_requests",
		Priority: prioBatchRequests,
		Dims: collectorapi.Dims{
			{ID: "batch_requests", Name: "batch", Algo: collectorapi.Incremental},
		},
	}
	compilationsChart = collectorapi.Chart{
		ID:       "compilations",
		Title:    "SQL compilations",
		Units:    "compilations/s",
		Fam:      "queries",
		Ctx:      "mssql.compilations",
		Priority: prioCompilations,
		Dims: collectorapi.Dims{
			{ID: "sql_compilations", Name: "compilations", Algo: collectorapi.Incremental},
		},
	}
	recompilationsChart = collectorapi.Chart{
		ID:       "recompilations",
		Title:    "SQL re-compilations",
		Units:    "recompilations/s",
		Fam:      "queries",
		Ctx:      "mssql.recompilations",
		Priority: prioRecompilations,
		Dims: collectorapi.Dims{
			{ID: "sql_recompilations", Name: "recompilations", Algo: collectorapi.Incremental},
		},
	}
	autoParamAttemptsChart = collectorapi.Chart{
		ID:       "auto_param_attempts",
		Title:    "Auto-parameterization attempts",
		Units:    "attempts/s",
		Fam:      "queries",
		Ctx:      "mssql.auto_param_attempts",
		Priority: prioAutoParamAttempts,
		Dims: collectorapi.Dims{
			{ID: "auto_param_attempts", Name: "total", Algo: collectorapi.Incremental},
			{ID: "auto_param_safe", Name: "safe", Algo: collectorapi.Incremental},
			{ID: "auto_param_failed", Name: "failed", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	sqlErrorsChart = collectorapi.Chart{
		ID:       "sql_errors",
		Title:    "SQL errors",
		Units:    "errors/s",
		Fam:      "errors",
		Ctx:      "mssql.sql_errors",
		Priority: prioSQLErrors,
		Dims: collectorapi.Dims{
			{ID: "sql_errors_total", Name: "errors", Algo: collectorapi.Incremental},
		},
	}

	bufferCacheHitRatioChart = collectorapi.Chart{
		ID:       "buffer_cache_hit_ratio",
		Title:    "Buffer cache hit ratio",
		Units:    "percentage",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_cache_hit_ratio",
		Priority: prioBufferCacheHitRatio,
		Dims: collectorapi.Dims{
			{ID: "buffer_cache_hit_ratio", Name: "hit_ratio"},
		},
	}
	bufferPageIOPSChart = collectorapi.Chart{
		ID:       "buffer_page_iops",
		Title:    "Buffer page I/O",
		Units:    "pages/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_page_iops",
		Priority: prioBufferPageIOPS,
		Dims: collectorapi.Dims{
			{ID: "buffer_page_reads", Name: "read", Algo: collectorapi.Incremental},
			{ID: "buffer_page_writes", Name: "written", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	bufferCheckpointPagesChart = collectorapi.Chart{
		ID:       "buffer_checkpoint_pages",
		Title:    "Buffer checkpoint pages flushed",
		Units:    "pages/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_checkpoint_pages",
		Priority: prioBufferCheckpointPages,
		Dims: collectorapi.Dims{
			{ID: "buffer_checkpoint_pages", Name: "flushed", Algo: collectorapi.Incremental},
		},
	}
	bufferPageLifeExpectancyChart = collectorapi.Chart{
		ID:       "buffer_page_life_expectancy",
		Title:    "Buffer page life expectancy",
		Units:    "seconds",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_page_life_expectancy",
		Priority: prioBufferPageLifeExpectancy,
		Dims: collectorapi.Dims{
			{ID: "buffer_page_life_expectancy", Name: "life_expectancy"},
		},
	}
	bufferLazyWritesChart = collectorapi.Chart{
		ID:       "buffer_lazy_writes",
		Title:    "Buffer lazy writes",
		Units:    "writes/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_lazy_writes",
		Priority: prioBufferLazyWrites,
		Dims: collectorapi.Dims{
			{ID: "buffer_lazy_writes", Name: "lazy_writes", Algo: collectorapi.Incremental},
		},
	}
	bufferPageLookupsChart = collectorapi.Chart{
		ID:       "buffer_page_lookups",
		Title:    "Buffer page lookups",
		Units:    "lookups/s",
		Fam:      "buffer",
		Ctx:      "mssql.buffer_page_lookups",
		Priority: prioBufferPageLookups,
		Dims: collectorapi.Dims{
			{ID: "buffer_page_lookups", Name: "lookups", Algo: collectorapi.Incremental},
		},
	}

	pageSplitsChart = collectorapi.Chart{
		ID:       "page_splits",
		Title:    "Page splits",
		Units:    "splits/s",
		Fam:      "access",
		Ctx:      "mssql.page_splits",
		Priority: prioPageSplits,
		Dims: collectorapi.Dims{
			{ID: "page_splits", Name: "page", Algo: collectorapi.Incremental},
		},
	}

	memoryTotalChart = collectorapi.Chart{
		ID:       "memory_total",
		Title:    "Total server memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mssql.memory_total",
		Priority: prioMemoryTotal,
		Dims: collectorapi.Dims{
			{ID: "memory_total", Name: "memory"},
		},
	}
	memoryConnectionChart = collectorapi.Chart{
		ID:       "memory_connection",
		Title:    "Connection memory",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "mssql.memory_connection",
		Priority: prioMemoryConnection,
		Dims: collectorapi.Dims{
			{ID: "memory_connection", Name: "memory"},
		},
	}
	memoryPendingGrantsChart = collectorapi.Chart{
		ID:       "memory_pending_grants",
		Title:    "Pending memory grants",
		Units:    "processes",
		Fam:      "memory",
		Ctx:      "mssql.memory_pending_grants",
		Priority: prioMemoryPendingGrants,
		Dims: collectorapi.Dims{
			{ID: "memory_pending_grants", Name: "pending"},
		},
	}
	memoryExternalBenefitChart = collectorapi.Chart{
		ID:       "memory_external_benefit",
		Title:    "External benefit of memory",
		Units:    "benefit",
		Fam:      "memory",
		Ctx:      "mssql.memory_external_benefit",
		Priority: prioMemoryExternalBenefit,
		Dims: collectorapi.Dims{
			{ID: "memory_external_benefit", Name: "benefit"},
		},
	}

	processMemoryResidentChart = collectorapi.Chart{
		ID:       "process_memory_resident",
		Title:    "Process resident memory (working set)",
		Units:    "bytes",
		Fam:      "process memory",
		Ctx:      "mssql.process_memory_resident",
		Priority: prioProcessMemoryResident,
		Dims: collectorapi.Dims{
			{ID: "process_memory_resident", Name: "resident"},
		},
	}
	processMemoryVirtualChart = collectorapi.Chart{
		ID:       "process_memory_virtual",
		Title:    "Process virtual memory committed",
		Units:    "bytes",
		Fam:      "process memory",
		Ctx:      "mssql.process_memory_virtual",
		Priority: prioProcessMemoryVirtual,
		Dims: collectorapi.Dims{
			{ID: "process_memory_virtual", Name: "virtual"},
		},
	}
	processMemoryUtilizationChart = collectorapi.Chart{
		ID:       "process_memory_utilization",
		Title:    "Process memory utilization",
		Units:    "percentage",
		Fam:      "process memory",
		Ctx:      "mssql.process_memory_utilization",
		Priority: prioProcessMemoryUtilization,
		Dims: collectorapi.Dims{
			{ID: "process_memory_utilization", Name: "utilization"},
		},
	}
	processPageFaultsChart = collectorapi.Chart{
		ID:       "process_page_faults",
		Title:    "Process page faults",
		Units:    "faults",
		Fam:      "process memory",
		Ctx:      "mssql.process_page_faults",
		Priority: prioProcessPageFaults,
		Dims: collectorapi.Dims{
			{ID: "process_page_faults", Name: "page_faults", Algo: collectorapi.Incremental},
		},
	}

	osMemoryChart = collectorapi.Chart{
		ID:       "os_memory",
		Title:    "OS physical memory",
		Units:    "bytes",
		Fam:      "os memory",
		Ctx:      "mssql.os_memory",
		Priority: prioOSMemory,
		Dims: collectorapi.Dims{
			{ID: "os_memory_used", Name: "used"},
			{ID: "os_memory_available", Name: "available"},
		},
	}
	osPageFileChart = collectorapi.Chart{
		ID:       "os_pagefile",
		Title:    "OS page file",
		Units:    "bytes",
		Fam:      "os memory",
		Ctx:      "mssql.os_pagefile",
		Priority: prioOSPageFile,
		Dims: collectorapi.Dims{
			{ID: "os_pagefile_used", Name: "used"},
			{ID: "os_pagefile_available", Name: "available"},
		},
	}
)

// Database chart templates
var (
	databaseActiveTransactionsChartTmpl = collectorapi.Chart{
		ID:       "database_%s_active_transactions",
		Title:    "Active transactions",
		Units:    "transactions",
		Fam:      "db transactions",
		Ctx:      "mssql.database_active_transactions",
		Priority: prioDatabaseActiveTransactions,
		Dims: collectorapi.Dims{
			{ID: "database_%s_active_transactions", Name: "active"},
		},
	}
	databaseTransactionsChartTmpl = collectorapi.Chart{
		ID:       "database_%s_transactions",
		Title:    "Transactions",
		Units:    "transactions/s",
		Fam:      "db transactions",
		Ctx:      "mssql.database_transactions",
		Priority: prioDatabaseTransactions,
		Dims: collectorapi.Dims{
			{ID: "database_%s_transactions", Name: "transactions", Algo: collectorapi.Incremental},
		},
	}
	databaseWriteTransactionsChartTmpl = collectorapi.Chart{
		ID:       "database_%s_write_transactions",
		Title:    "Write transactions",
		Units:    "transactions/s",
		Fam:      "db transactions",
		Ctx:      "mssql.database_write_transactions",
		Priority: prioDatabaseWriteTransactions,
		Dims: collectorapi.Dims{
			{ID: "database_%s_write_transactions", Name: "write", Algo: collectorapi.Incremental},
		},
	}
	databaseLogFlushesChartTmpl = collectorapi.Chart{
		ID:       "database_%s_log_flushes",
		Title:    "Log flushes",
		Units:    "flushes/s",
		Fam:      "db log",
		Ctx:      "mssql.database_log_flushes",
		Priority: prioDatabaseLogFlushes,
		Dims: collectorapi.Dims{
			{ID: "database_%s_log_flushes", Name: "flushes", Algo: collectorapi.Incremental},
		},
	}
	databaseLogFlushedChartTmpl = collectorapi.Chart{
		ID:       "database_%s_log_flushed",
		Title:    "Log bytes flushed",
		Units:    "bytes/s",
		Fam:      "db log",
		Ctx:      "mssql.database_log_flushed",
		Priority: prioDatabaseLogFlushed,
		Dims: collectorapi.Dims{
			{ID: "database_%s_log_flushed", Name: "flushed", Algo: collectorapi.Incremental},
		},
	}
	databaseDataFileSizeChartTmpl = collectorapi.Chart{
		ID:       "database_%s_data_file_size",
		Title:    "Data file size",
		Units:    "bytes",
		Fam:      "db size",
		Ctx:      "mssql.database_data_file_size",
		Priority: prioDatabaseDataFileSize,
		Dims: collectorapi.Dims{
			{ID: "database_%s_data_file_size", Name: "size"},
		},
	}
	databaseBackupRestoreThroughputChartTmpl = collectorapi.Chart{
		ID:       "database_%s_backup_restore_throughput",
		Title:    "Backup/Restore throughput",
		Units:    "bytes/s",
		Fam:      "db backup",
		Ctx:      "mssql.database_backup_restore_throughput",
		Priority: prioDatabaseDataFileSize + 1,
		Dims: collectorapi.Dims{
			{ID: "database_%s_backup_restore_throughput", Name: "throughput", Algo: collectorapi.Incremental},
		},
	}
	databaseStateChartTmpl = collectorapi.Chart{
		ID:       "database_%s_state",
		Title:    "Database state",
		Units:    "state",
		Fam:      "db status",
		Ctx:      "mssql.database_state",
		Priority: prioDatabaseState,
		Dims: collectorapi.Dims{
			{ID: "database_%s_state_online", Name: "online"},
			{ID: "database_%s_state_restoring", Name: "restoring"},
			{ID: "database_%s_state_recovering", Name: "recovering"},
			{ID: "database_%s_state_pending", Name: "pending"},
			{ID: "database_%s_state_suspect", Name: "suspect"},
			{ID: "database_%s_state_emergency", Name: "emergency"},
			{ID: "database_%s_state_offline", Name: "offline"},
		},
	}
	databaseReadOnlyChartTmpl = collectorapi.Chart{
		ID:       "database_%s_read_only",
		Title:    "Database read-only status",
		Units:    "status",
		Fam:      "db status",
		Ctx:      "mssql.database_read_only",
		Priority: prioDatabaseReadOnly,
		Dims: collectorapi.Dims{
			{ID: "database_%s_read_only", Name: "read_only"},
			{ID: "database_%s_read_write", Name: "read_write"},
		},
	}
	databaseLogGrowthsChartTmpl = collectorapi.Chart{
		ID:       "database_%s_log_growths",
		Title:    "Database log growths",
		Units:    "growths",
		Fam:      "db log",
		Ctx:      "mssql.database_log_growths",
		Priority: prioDatabaseLogGrowths,
		Dims: collectorapi.Dims{
			{ID: "database_%s_log_growths", Name: "growths", Algo: collectorapi.Incremental},
		},
	}
	databaseIOStallChartTmpl = collectorapi.Chart{
		ID:       "database_%s_io_stall",
		Title:    "Database I/O stall time",
		Units:    "ms",
		Fam:      "db io",
		Ctx:      "mssql.database_io_stall",
		Priority: prioDatabaseIOStall,
		Dims: collectorapi.Dims{
			{ID: "database_%s_io_stall_read", Name: "read", Algo: collectorapi.Incremental},
			{ID: "database_%s_io_stall_write", Name: "write", Algo: collectorapi.Incremental},
		},
	}
)

// Lock stats chart templates (per lock resource type, from performance counters)
var (
	lockStatsDeadlocksChartTmpl = collectorapi.Chart{
		ID:       "lock_stats_%s_deadlocks",
		Title:    "Deadlocks by lock resource type",
		Units:    "deadlocks/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_deadlocks",
		Priority: prioDatabaseDeadlocks,
		Dims: collectorapi.Dims{
			{ID: "lock_stats_%s_deadlocks", Name: "deadlocks", Algo: collectorapi.Incremental},
		},
	}
	lockStatsWaitsChartTmpl = collectorapi.Chart{
		ID:       "lock_stats_%s_waits",
		Title:    "Lock waits by lock resource type",
		Units:    "waits/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_waits",
		Priority: prioDatabaseLockWaits,
		Dims: collectorapi.Dims{
			{ID: "lock_stats_%s_waits", Name: "waits", Algo: collectorapi.Incremental},
		},
	}
	lockStatsTimeoutsChartTmpl = collectorapi.Chart{
		ID:       "lock_stats_%s_timeouts",
		Title:    "Lock timeouts by lock resource type",
		Units:    "timeouts/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_timeouts",
		Priority: prioDatabaseLockTimeouts,
		Dims: collectorapi.Dims{
			{ID: "lock_stats_%s_timeouts", Name: "timeouts", Algo: collectorapi.Incremental},
		},
	}
	lockStatsRequestsChartTmpl = collectorapi.Chart{
		ID:       "lock_stats_%s_requests",
		Title:    "Lock requests by lock resource type",
		Units:    "requests/s",
		Fam:      "lock stats",
		Ctx:      "mssql.lock_stats_requests",
		Priority: prioDatabaseLockRequests,
		Dims: collectorapi.Dims{
			{ID: "lock_stats_%s_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
)

// Lock resource chart template
var locksByResourceChartTmpl = collectorapi.Chart{
	ID:       "locks_by_resource_%s",
	Title:    "Locks by resource type",
	Units:    "locks",
	Fam:      "locks",
	Ctx:      "mssql.locks_by_resource",
	Priority: prioLocksByResource,
	Dims: collectorapi.Dims{
		{ID: "locks_%s_count", Name: "locks"},
	},
}

// Wait type chart templates
var (
	waitTotalTimeChartTmpl = collectorapi.Chart{
		ID:       "wait_%s_total_time",
		Title:    "Total wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_total_time",
		Priority: prioWaitTotalTime,
		Dims: collectorapi.Dims{
			{ID: "wait_%s_total_ms", Name: "duration", Algo: collectorapi.Incremental},
		},
	}
	waitResourceTimeChartTmpl = collectorapi.Chart{
		ID:       "wait_%s_resource_time",
		Title:    "Resource wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_resource_time",
		Priority: prioWaitResourceTime,
		Dims: collectorapi.Dims{
			{ID: "wait_%s_resource_ms", Name: "duration", Algo: collectorapi.Incremental},
		},
	}
	waitSignalTimeChartTmpl = collectorapi.Chart{
		ID:       "wait_%s_signal_time",
		Title:    "Signal wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_signal_time",
		Priority: prioWaitSignalTime,
		Dims: collectorapi.Dims{
			{ID: "wait_%s_signal_ms", Name: "duration", Algo: collectorapi.Incremental},
		},
	}
	waitCountChartTmpl = collectorapi.Chart{
		ID:       "wait_%s_count",
		Title:    "Wait count",
		Units:    "waits/s",
		Fam:      "waits",
		Ctx:      "mssql.wait_count",
		Priority: prioWaitCount,
		Dims: collectorapi.Dims{
			{ID: "wait_%s_tasks", Name: "waits", Algo: collectorapi.Incremental},
		},
	}
	waitMaxTimeChartTmpl = collectorapi.Chart{
		ID:       "wait_%s_max_time",
		Title:    "Maximum wait time",
		Units:    "ms",
		Fam:      "waits",
		Ctx:      "mssql.wait_max_time",
		Priority: prioWaitMaxTime,
		Dims: collectorapi.Dims{
			{ID: "wait_%s_max_ms", Name: "max_time"},
		},
	}
)

// Job status chart template
var jobStatusChartTmpl = collectorapi.Chart{
	ID:       "job_%s_status",
	Title:    "Job status",
	Units:    "status",
	Fam:      "jobs",
	Ctx:      "mssql.job_status",
	Priority: prioJobStatus,
	Dims: collectorapi.Dims{
		{ID: "job_%s_enabled", Name: "enabled"},
		{ID: "job_%s_disabled", Name: "disabled"},
	},
}

// Replication chart templates
var (
	replicationStatusChartTmpl = collectorapi.Chart{
		ID:       "replication_%s_status",
		Title:    "Replication status",
		Units:    "status",
		Fam:      "replication",
		Ctx:      "mssql.replication_status",
		Priority: prioReplicationStatus,
		Dims: collectorapi.Dims{
			{ID: "replication_%s_status_started", Name: "started"},
			{ID: "replication_%s_status_succeeded", Name: "succeeded"},
			{ID: "replication_%s_status_in_progress", Name: "in_progress"},
			{ID: "replication_%s_status_idle", Name: "idle"},
			{ID: "replication_%s_status_retrying", Name: "retrying"},
			{ID: "replication_%s_status_failed", Name: "failed"},
		},
	}
	replicationWarningChartTmpl = collectorapi.Chart{
		ID:       "replication_%s_warning",
		Title:    "Replication warnings",
		Units:    "flags",
		Fam:      "replication",
		Ctx:      "mssql.replication_warning",
		Priority: prioReplicationStatus + 1,
		Dims: collectorapi.Dims{
			{ID: "replication_%s_warning_expiration", Name: "expiration"},
			{ID: "replication_%s_warning_latency", Name: "latency"},
			{ID: "replication_%s_warning_mergeexpiration", Name: "merge_expiration"},
			{ID: "replication_%s_warning_mergeslowrunduration", Name: "merge_slow_duration"},
			{ID: "replication_%s_warning_mergefastrunduration", Name: "merge_fast_duration"},
			{ID: "replication_%s_warning_mergefastrunspeed", Name: "merge_fast_speed"},
			{ID: "replication_%s_warning_mergeslowrunspeed", Name: "merge_slow_speed"},
		},
	}
	replicationLatencyChartTmpl = collectorapi.Chart{
		ID:       "replication_%s_latency",
		Title:    "Replication latency",
		Units:    "seconds",
		Fam:      "replication",
		Ctx:      "mssql.replication_latency",
		Priority: prioReplicationLatency,
		Dims: collectorapi.Dims{
			{ID: "replication_%s_latency_avg", Name: "average"},
			{ID: "replication_%s_latency_best", Name: "best"},
			{ID: "replication_%s_latency_worst", Name: "worst"},
		},
	}
	replicationSubscriptionsChartTmpl = collectorapi.Chart{
		ID:       "replication_%s_subscriptions",
		Title:    "Replication subscriptions",
		Units:    "subscriptions",
		Fam:      "replication",
		Ctx:      "mssql.replication_subscriptions",
		Priority: prioReplicationStatus + 1,
		Dims: collectorapi.Dims{
			{ID: "replication_%s_subscriptions", Name: "total"},
			{ID: "replication_%s_agents_running", Name: "agents_running"},
		},
	}
)

func (c *Collector) addDatabaseCharts(dbName string) {
	// internal databases used by SQL Server's engine
	// Their metrics don't represent actual workload or user activity
	if strings.HasPrefix(dbName, "model_") {
		return
	}

	charts := &collectorapi.Charts{
		databaseActiveTransactionsChartTmpl.Copy(),
		databaseTransactionsChartTmpl.Copy(),
		databaseWriteTransactionsChartTmpl.Copy(),
		databaseLogFlushesChartTmpl.Copy(),
		databaseLogFlushedChartTmpl.Copy(),
		databaseLogGrowthsChartTmpl.Copy(),
		databaseIOStallChartTmpl.Copy(),
		databaseDataFileSizeChartTmpl.Copy(),
		databaseBackupRestoreThroughputChartTmpl.Copy(),
		databaseStateChartTmpl.Copy(),
		databaseReadOnlyChartTmpl.Copy(),
	}

	dbID := cleanDatabaseName(dbName)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dbID)
		chart.Labels = []collectorapi.Label{
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
	charts := &collectorapi.Charts{
		waitTotalTimeChartTmpl.Copy(),
		waitResourceTimeChartTmpl.Copy(),
		waitSignalTimeChartTmpl.Copy(),
		waitMaxTimeChartTmpl.Copy(),
		waitCountChartTmpl.Copy(),
	}

	waitID := cleanWaitTypeName(waitType)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, waitID)
		chart.Labels = []collectorapi.Label{
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
	chart.Labels = []collectorapi.Label{
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
	charts := &collectorapi.Charts{
		lockStatsDeadlocksChartTmpl.Copy(),
		lockStatsWaitsChartTmpl.Copy(),
		lockStatsTimeoutsChartTmpl.Copy(),
		lockStatsRequestsChartTmpl.Copy(),
	}

	resID := cleanResourceTypeName(resourceType)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, resID)
		chart.Labels = []collectorapi.Label{
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
	chart.Labels = []collectorapi.Label{
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
	charts := &collectorapi.Charts{
		replicationStatusChartTmpl.Copy(),
		replicationWarningChartTmpl.Copy(),
		replicationLatencyChartTmpl.Copy(),
		replicationSubscriptionsChartTmpl.Copy(),
	}

	pubID := cleanPublicationName(pubDB, publication)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, pubID)
		chart.Labels = []collectorapi.Label{
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
