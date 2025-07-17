// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		// Overview charts (Screen 01)
		databaseCountChart.Copy(),
		cpuUsageChart.Copy(),
		activeConnectionsChart.Copy(),
		memoryUsageChart.Copy(),
		sqlStatementsChart.Copy(),
		transactionActivityChart.Copy(),
		timeSpentChart.Copy(),
		
		// Existing charts
		serviceHealthChart.Copy(),
		connectionsChart.Copy(),
		lockingChart.Copy(),
		lockDetailsChart.Copy(),
		lockWaitTimeChart.Copy(),
		deadlocksChart.Copy(),
		sortingChart.Copy(),
		rowActivityChart.Copy(),
		bufferpoolHitRatioChart.Copy(),
		bufferpoolDataHitRatioChart.Copy(),
		bufferpoolIndexHitRatioChart.Copy(),
		bufferpoolXDAHitRatioChart.Copy(),
		bufferpoolColumnHitRatioChart.Copy(),
		bufferpoolReadsChart.Copy(),
		bufferpoolDataReadsChart.Copy(),
		bufferpoolIndexReadsChart.Copy(),
		bufferpoolXDAReadsChart.Copy(),
		bufferpoolColumnReadsChart.Copy(),
		
		// Enhanced logging charts (Screen 18)
		logOperationsChart.Copy(),
		logTimingChart.Copy(),
		logBufferEventsChart.Copy(),
		
		// Existing log charts
		logSpaceChart.Copy(),
		logUtilizationChart.Copy(),
		logIOChart.Copy(),
		longRunningQueriesChart.Copy(),
		backupStatusChart.Copy(),
		backupAgeChart.Copy(),
		
		// Federation charts (Screen 32)
		federationConnectionsChart.Copy(),
		federationOperationsChart.Copy(),
	}
)

var (
	serviceHealthChart = module.Chart{
		ID:       "service_health",
		Title:    "Service Health Status",
		Units:    "status",
		Fam:      "overview",
		Ctx:      "db2.service_health",
		Priority: module.Priority - 10,
		Dims: module.Dims{
			{ID: "can_connect", Name: "connection"},
			{ID: "database_status", Name: "database"},
		},
	}

	connectionsChart = module.Chart{
		ID:       "connections",
		Title:    "Database Connections",
		Units:    "connections",
		Fam:      "connections/overview",
		Ctx:      "db2.connections",
		Priority: module.Priority,
		Dims: module.Dims{
			{ID: "conn_total", Name: "total"},
			{ID: "conn_active", Name: "active"},
			{ID: "conn_executing", Name: "executing"},
			{ID: "conn_idle", Name: "idle"},
			{ID: "conn_max", Name: "max_allowed"},
		},
	}

	lockingChart = module.Chart{
		ID:       "locking",
		Title:    "Database Locking",
		Units:    "events/s",
		Fam:      "locking",
		Ctx:      "db2.locking",
		Priority: module.Priority + 10,
		Dims: module.Dims{
			{ID: "lock_waits", Name: "waits", Algo: module.Incremental},
			{ID: "lock_timeouts", Name: "timeouts", Algo: module.Incremental},
			{ID: "lock_escalations", Name: "escalations", Algo: module.Incremental},
		},
	}

	deadlocksChart = module.Chart{
		ID:       "deadlocks",
		Title:    "Database Deadlocks",
		Units:    "deadlocks/s",
		Fam:      "locking",
		Ctx:      "db2.deadlocks",
		Priority: module.Priority + 11,
		Dims: module.Dims{
			{ID: "deadlocks", Name: "deadlocks", Algo: module.Incremental},
		},
	}

	sortingChart = module.Chart{
		ID:       "sorting",
		Title:    "Database Sorting",
		Units:    "sorts/s",
		Fam:      "activity/sorting",
		Ctx:      "db2.sorting",
		Priority: module.Priority + 20,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "total_sorts", Name: "sorts", Algo: module.Incremental},
			{ID: "sort_overflows", Name: "overflows", Algo: module.Incremental},
		},
	}

	lockDetailsChart = module.Chart{
		ID:       "lock_details",
		Title:    "Lock Details",
		Units:    "locks",
		Fam:      "locking",
		Ctx:      "db2.lock_details",
		Priority: module.Priority + 12,
		Dims: module.Dims{
			{ID: "lock_active", Name: "active"},
			{ID: "lock_waiting_agents", Name: "waiting_agents"},
			{ID: "lock_memory_pages", Name: "memory_pages"},
		},
	}

	lockWaitTimeChart = module.Chart{
		ID:       "lock_wait_time",
		Title:    "Average Lock Wait Time",
		Units:    "milliseconds",
		Fam:      "locking",
		Ctx:      "db2.lock_wait_time",
		Priority: module.Priority + 13,
		Dims: module.Dims{
			{ID: "lock_wait_time", Name: "wait_time", Div: Precision},
		},
	}

	rowActivityChart = module.Chart{
		ID:       "row_activity",
		Title:    "Row Activity",
		Units:    "rows/s",
		Fam:      "activity/rows",
		Ctx:      "db2.row_activity",
		Priority: module.Priority + 30,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "rows_read", Name: "read", Algo: module.Incremental},
			{ID: "rows_returned", Name: "returned", Algo: module.Incremental},
			{ID: "rows_modified", Name: "modified", Algo: module.Incremental, Mul: -1},
		},
	}

	bufferpoolHitRatioChart = module.Chart{
		ID:       "bufferpool_hit_ratio",
		Title:    "Buffer Pool Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpools/overview",
		Ctx:      "db2.bufferpool_hit_ratio",
		Priority: module.Priority + 40,
		Dims: module.Dims{
			{ID: "bufferpool_hits", Name: "hits", Algo: module.PercentOfIncremental},
			{ID: "bufferpool_misses", Name: "misses", Algo: module.PercentOfIncremental},
		},
	}

	// Note: We'll need separate charts for each type's hit ratio
	// since percentage-of-incremental-row requires all dimensions to sum to 100%
	bufferpoolDataHitRatioChart = module.Chart{
		ID:       "bufferpool_data_hit_ratio",
		Title:    "Buffer Pool Data Page Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpools/data",
		Ctx:      "db2.bufferpool_data_hit_ratio",
		Priority: module.Priority + 41,
		Dims: module.Dims{
			{ID: "bufferpool_data_hits", Name: "hits", Algo: module.PercentOfIncremental},
			{ID: "bufferpool_data_misses", Name: "misses", Algo: module.PercentOfIncremental},
		},
	}

	bufferpoolIndexHitRatioChart = module.Chart{
		ID:       "bufferpool_index_hit_ratio",
		Title:    "Buffer Pool Index Page Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpools/index",
		Ctx:      "db2.bufferpool_index_hit_ratio",
		Priority: module.Priority + 42,
		Dims: module.Dims{
			{ID: "bufferpool_index_hits", Name: "hits", Algo: module.PercentOfIncremental},
			{ID: "bufferpool_index_misses", Name: "misses", Algo: module.PercentOfIncremental},
		},
	}

	bufferpoolXDAHitRatioChart = module.Chart{
		ID:       "bufferpool_xda_hit_ratio",
		Title:    "Buffer Pool XDA Page Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpools/xda",
		Ctx:      "db2.bufferpool_xda_hit_ratio",
		Priority: module.Priority + 43,
		Dims: module.Dims{
			{ID: "bufferpool_xda_hits", Name: "hits", Algo: module.PercentOfIncremental},
			{ID: "bufferpool_xda_misses", Name: "misses", Algo: module.PercentOfIncremental},
		},
	}

	bufferpoolColumnHitRatioChart = module.Chart{
		ID:       "bufferpool_column_hit_ratio",
		Title:    "Buffer Pool Column Page Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpools/columns",
		Ctx:      "db2.bufferpool_column_hit_ratio",
		Priority: module.Priority + 44,
		Dims: module.Dims{
			{ID: "bufferpool_column_hits", Name: "hits", Algo: module.PercentOfIncremental},
			{ID: "bufferpool_column_misses", Name: "misses", Algo: module.PercentOfIncremental},
		},
	}

	bufferpoolReadsChart = module.Chart{
		ID:       "bufferpool_reads",
		Title:    "Buffer Pool Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/overview",
		Ctx:      "db2.bufferpool_reads",
		Priority: module.Priority + 45,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolDataReadsChart = module.Chart{
		ID:       "bufferpool_data_reads",
		Title:    "Buffer Pool Data Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/data",
		Ctx:      "db2.bufferpool_data_reads",
		Priority: module.Priority + 43,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_data_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_data_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolIndexReadsChart = module.Chart{
		ID:       "bufferpool_index_reads",
		Title:    "Buffer Pool Index Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/index",
		Ctx:      "db2.bufferpool_index_reads",
		Priority: module.Priority + 44,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_index_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_index_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolXDAReadsChart = module.Chart{
		ID:       "bufferpool_xda_reads",
		Title:    "Buffer Pool XDA Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/xda",
		Ctx:      "db2.bufferpool_xda_reads",
		Priority: module.Priority + 45,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_xda_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_xda_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolColumnReadsChart = module.Chart{
		ID:       "bufferpool_column_reads",
		Title:    "Buffer Pool Column Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/columns",
		Ctx:      "db2.bufferpool_column_reads",
		Priority: module.Priority + 46,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_column_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_column_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	logSpaceChart = module.Chart{
		ID:       "log_space",
		Title:    "Log Space Usage",
		Units:    "bytes",
		Fam:      "storage/space",
		Ctx:      "db2.log_space",
		Priority: module.Priority + 50,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "log_used_space", Name: "used"},
			{ID: "log_available_space", Name: "available"},
		},
	}

	logUtilizationChart = module.Chart{
		ID:       "log_utilization",
		Title:    "Log Space Utilization",
		Units:    "percentage",
		Fam:      "storage/space",
		Ctx:      "db2.log_utilization",
		Priority: module.Priority + 51,
		Dims: module.Dims{
			{ID: "log_utilization", Name: "utilization", Div: Precision},
		},
	}

	logIOChart = module.Chart{
		ID:       "log_io",
		Title:    "Log I/O Operations",
		Units:    "operations/s",
		Fam:      "storage/operations",
		Ctx:      "db2.log_io",
		Priority: module.Priority + 52,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "log_io_reads", Name: "reads", Algo: module.Incremental},
			{ID: "log_io_writes", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}

	longRunningQueriesChart = module.Chart{
		ID:       "long_running_queries",
		Title:    "Long Running Queries",
		Units:    "queries",
		Fam:      "activity/requests",
		Ctx:      "db2.long_running_queries",
		Priority: module.Priority + 60,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "long_running_queries", Name: "total"},
			{ID: "long_running_queries_warning", Name: "warning"},
			{ID: "long_running_queries_critical", Name: "critical"},
		},
	}

	backupStatusChart = module.Chart{
		ID:       "backup_status",
		Title:    "Last Backup Status",
		Units:    "status",
		Fam:      "storage/backup",
		Ctx:      "db2.backup_status",
		Priority: module.Priority + 70,
		Dims: module.Dims{
			{ID: "last_backup_status", Name: "status"},
		},
	}

	backupAgeChart = module.Chart{
		ID:       "backup_age",
		Title:    "Time Since Last Backup",
		Units:    "hours",
		Fam:      "storage/backup",
		Ctx:      "db2.backup_age",
		Priority: module.Priority + 71,
		Dims: module.Dims{
			{ID: "last_full_backup_age", Name: "full"},
			{ID: "last_incremental_backup_age", Name: "incremental"},
		},
	}

	// Database Overview charts (Screen 01)
	databaseCountChart = module.Chart{
		ID:       "database_count",
		Title:    "Database Count",
		Units:    "databases",
		Fam:      "overview",
		Ctx:      "db2.database_count",
		Priority: module.Priority - 100,
		Dims: module.Dims{
			{ID: "database_active", Name: "active"},
			{ID: "database_inactive", Name: "inactive"},
		},
	}

	cpuUsageChart = module.Chart{
		ID:       "cpu_usage",
		Title:    "CPU Usage (100% = 1 CPU core)",
		Units:    "percentage",
		Fam:      "overview",
		Ctx:      "db2.cpu_usage",
		Priority: module.Priority - 99,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "cpu_user", Name: "user", Div: Precision},
			{ID: "cpu_system", Name: "system", Div: Precision},
			{ID: "cpu_idle", Name: "idle", Div: Precision},
			{ID: "cpu_iowait", Name: "iowait", Div: Precision},
		},
	}

	activeConnectionsChart = module.Chart{
		ID:       "active_connections",
		Title:    "Active Connections",
		Units:    "connections",
		Fam:      "connections/overview",
		Ctx:      "db2.active_connections",
		Priority: module.Priority - 98,
		Dims: module.Dims{
			{ID: "connections_active", Name: "active"},
			{ID: "connections_total", Name: "total"},
		},
	}

	memoryUsageChart = module.Chart{
		ID:       "memory_usage",
		Title:    "Memory Usage",
		Units:    "MB",
		Fam:      "overview",
		Ctx:      "db2.memory_usage",
		Priority: module.Priority - 97,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "memory_database_committed", Name: "database", Div: 1048576},
			{ID: "memory_instance_committed", Name: "instance", Div: 1048576},
			{ID: "memory_bufferpool_used", Name: "bufferpool", Div: 1048576},
			{ID: "memory_shared_sort_used", Name: "shared_sort", Div: 1048576},
		},
	}

	// Split into additive charts following RULE #1: Non-Overlapping Dimensions
	sqlStatementsChart = module.Chart{
		ID:       "sql_statements",
		Title:    "SQL Statements",
		Units:    "statements/s",
		Fam:      "activity/requests",
		Ctx:      "db2.sql_statements", 
		Priority: module.Priority - 96,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "ops_select_stmts", Name: "selects", Algo: module.Incremental},
			{ID: "ops_uid_stmts", Name: "modifications", Algo: module.Incremental},
		},
	}

	transactionActivityChart = module.Chart{
		ID:       "transaction_activity",
		Title:    "Transaction Activity", 
		Units:    "transactions/s",
		Fam:      "activity/transactions",
		Ctx:      "db2.transaction_activity",
		Priority: module.Priority - 95,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "ops_transactions", Name: "committed", Algo: module.Incremental},
			{ID: "ops_activities_aborted", Name: "aborted", Algo: module.Incremental},
		},
	}

	timeSpentChart = module.Chart{
		ID:       "time_spent",
		Title:    "Average Operation Times",
		Units:    "milliseconds",
		Fam:      "activity/time-spent",
		Ctx:      "db2.time_spent",
		Priority: module.Priority - 95,
		Dims: module.Dims{
			{ID: "time_avg_direct_read", Name: "direct_read", Div: 1000},
			{ID: "time_avg_direct_write", Name: "direct_write", Div: 1000},
			{ID: "time_avg_pool_read", Name: "pool_read", Div: 1000},
			{ID: "time_avg_pool_write", Name: "pool_write", Div: 1000},
		},
	}

	// Enhanced logging charts (Screen 18)
	logOperationsChart = module.Chart{
		ID:       "log_operations",
		Title:    "Log Operations",
		Units:    "operations/s",
		Fam:      "storage/operations",
		Ctx:      "db2.log_operations",
		Priority: module.Priority + 53,
		Dims: module.Dims{
			{ID: "log_commits", Name: "commits", Algo: module.Incremental},
			{ID: "log_rollbacks", Name: "rollbacks", Algo: module.Incremental},
			{ID: "log_op_reads", Name: "reads", Algo: module.Incremental},
			{ID: "log_op_writes", Name: "writes", Algo: module.Incremental},
		},
	}

	logTimingChart = module.Chart{
		ID:       "log_timing",
		Title:    "Log Operation Times",
		Units:    "milliseconds",
		Fam:      "storage/operations",
		Ctx:      "db2.log_timing",
		Priority: module.Priority + 54,
		Dims: module.Dims{
			{ID: "log_avg_commit_time", Name: "avg_commit", Div: 1000},
			{ID: "log_avg_read_time", Name: "avg_read", Div: 1000},
			{ID: "log_avg_write_time", Name: "avg_write", Div: 1000},
		},
	}

	logBufferEventsChart = module.Chart{
		ID:       "log_buffer_events",
		Title:    "Log Buffer Full Events",
		Units:    "events/s",
		Fam:      "storage/operations",
		Ctx:      "db2.log_buffer_events",
		Priority: module.Priority + 55,
		Dims: module.Dims{
			{ID: "log_buffer_full_events", Name: "buffer_full", Algo: module.Incremental},
		},
	}

	// Federation charts (Screen 32)
	federationConnectionsChart = module.Chart{
		ID:       "federation_connections",
		Title:    "Federated Connections",
		Units:    "connections",
		Fam:      "federation",
		Ctx:      "db2.federation_connections",
		Priority: module.Priority + 80,
		Dims: module.Dims{
			{ID: "fed_connections_active", Name: "active"},
			{ID: "fed_connections_idle", Name: "idle"},
		},
	}

	federationOperationsChart = module.Chart{
		ID:       "federation_operations",
		Title:    "Federated Operations",
		Units:    "operations/s",
		Fam:      "federation",
		Ctx:      "db2.federation_operations",
		Priority: module.Priority + 81,
		Dims: module.Dims{
			{ID: "fed_rows_read", Name: "rows_read", Algo: module.Incremental},
			{ID: "fed_select_stmts", Name: "selects", Algo: module.Incremental},
			{ID: "fed_waits_total", Name: "waits", Algo: module.Incremental},
		},
	}
)
