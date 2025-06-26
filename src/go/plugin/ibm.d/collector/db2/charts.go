// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	baseCharts = module.Charts{
		serviceHealthChart.Copy(),
		connectionsChart.Copy(),
		lockingChart.Copy(),
		lockDetailsChart.Copy(),
		lockWaitTimeChart.Copy(),
		deadlocksChart.Copy(),
		sortingChart.Copy(),
		rowActivityChart.Copy(),
		bufferpoolHitRatioChart.Copy(),
		bufferpoolDetailedHitRatioChart.Copy(),
		bufferpoolReadsChart.Copy(),
		bufferpoolDataReadsChart.Copy(),
		bufferpoolIndexReadsChart.Copy(),
		bufferpoolXDAReadsChart.Copy(),
		bufferpoolColumnReadsChart.Copy(),
		logSpaceChart.Copy(),
		logUtilizationChart.Copy(),
		logIOChart.Copy(),
		longRunningQueriesChart.Copy(),
		backupStatusChart.Copy(),
		backupAgeChart.Copy(),
	}
)

var (
	serviceHealthChart = module.Chart{
		ID:       "service_health",
		Title:    "Service Health Status",
		Units:    "status",
		Fam:      "health",
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
		Fam:      "connections",
		Ctx:      "db2.connections",
		Priority: module.Priority,
		Dims: module.Dims{
			{ID: "conn_total", Name: "total"},
			{ID: "conn_active", Name: "active"},
			{ID: "conn_executing", Name: "executing"},
			{ID: "conn_idle", Name: "idle"},
			{ID: "conn_max", Name: "max_seen"},
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
		Fam:      "performance",
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
			{ID: "lock_wait_time", Name: "wait_time", Div: precision},
		},
	}

	rowActivityChart = module.Chart{
		ID:       "row_activity",
		Title:    "Row Activity",
		Units:    "rows/s",
		Fam:      "activity",
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
		Fam:      "performance",
		Ctx:      "db2.bufferpool_hit_ratio",
		Priority: module.Priority + 40,
		Dims: module.Dims{
			{ID: "bufferpool_hit_ratio", Name: "hit_ratio", Div: precision},
		},
	}

	bufferpoolDetailedHitRatioChart = module.Chart{
		ID:       "bufferpool_detailed_hit_ratio",
		Title:    "Buffer Pool Detailed Hit Ratios",
		Units:    "percentage",
		Fam:      "performance",
		Ctx:      "db2.bufferpool_detailed_hit_ratio",
		Priority: module.Priority + 41,
		Dims: module.Dims{
			{ID: "bufferpool_data_hit_ratio", Name: "data", Div: precision},
			{ID: "bufferpool_index_hit_ratio", Name: "index", Div: precision},
			{ID: "bufferpool_xda_hit_ratio", Name: "xda", Div: precision},
			{ID: "bufferpool_column_hit_ratio", Name: "column", Div: precision},
		},
	}
	
	bufferpoolReadsChart = module.Chart{
		ID:       "bufferpool_reads",
		Title:    "Buffer Pool Reads",
		Units:    "reads/s",
		Fam:      "performance",
		Ctx:      "db2.bufferpool_reads",
		Priority: module.Priority + 42,
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
		Fam:      "performance",
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
		Fam:      "performance",
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
		Fam:      "performance",
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
		Fam:      "performance",
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
		Fam:      "storage",
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
		Fam:      "storage",
		Ctx:      "db2.log_utilization",
		Priority: module.Priority + 51,
		Dims: module.Dims{
			{ID: "log_utilization", Name: "utilization", Div: precision},
		},
	}
	
	logIOChart = module.Chart{
		ID:       "log_io",
		Title:    "Log I/O Operations",
		Units:    "operations/s",
		Fam:      "storage",
		Ctx:      "db2.log_io",
		Priority: module.Priority + 52,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "log_reads", Name: "reads", Algo: module.Incremental},
			{ID: "log_writes", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}

	longRunningQueriesChart = module.Chart{
		ID:       "long_running_queries",
		Title:    "Long Running Queries",
		Units:    "queries",
		Fam:      "performance",
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
		Fam:      "backup",
		Ctx:      "db2.backup_status",
		Priority: module.Priority + 70,
		Dims: module.Dims{
			{ID: "last_backup_status", Name: "status"},
		},
	}

	backupAgeChart = module.Chart{
		ID:       "backup_age",
		Title:    "Time Since Last Backup",
		Units:    "seconds",
		Fam:      "backup",
		Ctx:      "db2.backup_age",
		Priority: module.Priority + 71,
		Dims: module.Dims{
			{ID: "last_full_backup_age", Name: "full", Mul: 3600},
			{ID: "last_incremental_backup_age", Name: "incremental", Mul: 3600},
		},
	}
)
