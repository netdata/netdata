// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	prioConnections                       = module.Priority + 1
	prioLocking                          = module.Priority + 2
	prioDeadlocks                        = module.Priority + 3
	prioSorting                          = module.Priority + 4
	prioRowActivity                      = module.Priority + 5
	prioBufferpoolHitRatio               = module.Priority + 6
	prioBufferpoolDetailedHitRatio       = module.Priority + 7
	prioBufferpoolReads                  = module.Priority + 8
	prioBufferpoolDataReads              = module.Priority + 9
	prioBufferpoolIndexReads             = module.Priority + 10
	prioBufferpoolXDAReads               = module.Priority + 11
	prioBufferpoolColumnReads            = module.Priority + 12
	prioLogSpace                         = module.Priority + 13
	prioLogUtilization                   = module.Priority + 14
	prioLogIO                            = module.Priority + 15
	prioLongRunningQueries               = module.Priority + 16
	prioBackupStatus                     = module.Priority + 17
	prioBackupAge                        = module.Priority + 18
	prioServiceHealth                    = module.Priority + 19
	prioLockDetails                      = module.Priority + 20
	prioLockWaitTime                     = module.Priority + 21
	prioDatabaseStatus                   = module.Priority + 100
	prioDatabaseApplications             = module.Priority + 101
	prioBufferpoolHitRatioInstance       = module.Priority + 110
	prioBufferpoolDetailedHitRatioInstance = module.Priority + 111
	prioBufferpoolReadsInstance          = module.Priority + 112
	prioBufferpoolDataReadsInstance      = module.Priority + 113
	prioBufferpoolIndexReadsInstance     = module.Priority + 114
	prioBufferpoolPagesInstance          = module.Priority + 115
	prioBufferpoolWritesInstance         = module.Priority + 116
	prioTablespaceUsage                  = module.Priority + 120
	prioTablespaceSize                   = module.Priority + 121
	prioTablespaceUsableSize             = module.Priority + 122
	prioTablespaceState                  = module.Priority + 123
	prioConnectionState                  = module.Priority + 130
	prioConnectionActivity               = module.Priority + 131
	prioConnectionWaitTime               = module.Priority + 132
	prioConnectionProcessingTime         = module.Priority + 133
	prioTableSize                        = module.Priority + 140
	prioTableActivity                    = module.Priority + 141
	prioIndexUsage                       = module.Priority + 150
	prioMemoryPoolUsage                  = module.Priority + 160
	prioMemoryPoolHWM                    = module.Priority + 161
	prioTableIOScans                     = module.Priority + 170
	prioTableIORows                      = module.Priority + 171
	prioTableIOActivity                  = module.Priority + 172
	prioMemorySetUsage                   = module.Priority + 180
	prioMemorySetCommitted               = module.Priority + 181
	prioMemorySetHighWaterMark           = module.Priority + 182
	prioMemorySetAdditionalCommitted     = module.Priority + 183
	prioMemorySetPercentUsedHWM          = module.Priority + 184
	prioPrefetcherPrefetchRatio          = module.Priority + 190
	prioPrefetcherCleanerRatio           = module.Priority + 191
	prioPrefetcherPhysicalReads          = module.Priority + 192
	prioPrefetcherAsyncReads             = module.Priority + 193
	prioPrefetcherWaitTime               = module.Priority + 194
	prioPrefetcherUnreadPages            = module.Priority + 195
)

const (
	nodeIDLabelKey = "node_id"
)

// Global charts
var globalCharts = module.Charts{
	{
		ID:       "connections",
		Title:    "Database Connections",
		Units:    "connections",
		Fam:      "connections/instances",
		Ctx:      "db2.connections",
		Type:     module.Line,
		Priority: prioConnections,
		Dims: module.Dims{
			{ID: "conn_total", Name: "total"},
			{ID: "conn_active", Name: "active"},
			{ID: "conn_executing", Name: "executing"},
			{ID: "conn_idle", Name: "idle"},
			{ID: "conn_max", Name: "max_allowed"},
		},
	},
	{
		ID:       "locking",
		Title:    "Database Locking",
		Units:    "events/s",
		Fam:      "locking",
		Ctx:      "db2.locking",
		Type:     module.Line,
		Priority: prioLocking,
		Dims: module.Dims{
			{ID: "lock_waits", Name: "waits", Algo: module.Incremental},
			{ID: "lock_timeouts", Name: "timeouts", Algo: module.Incremental},
			{ID: "lock_escalations", Name: "escalations", Algo: module.Incremental},
		},
	},
	{
		ID:       "deadlocks",
		Title:    "Database Deadlocks",
		Units:    "deadlocks/s",
		Fam:      "locking",
		Ctx:      "db2.deadlocks",
		Type:     module.Line,
		Priority: prioDeadlocks,
		Dims: module.Dims{
			{ID: "deadlocks", Name: "deadlocks", Algo: module.Incremental},
		},
	},
	{
		ID:       "sorting",
		Title:    "Database Sorting",
		Units:    "sorts/s",
		Fam:      "performance",
		Ctx:      "db2.sorting",
		Type:     module.Stacked,
		Priority: prioSorting,
		Dims: module.Dims{
			{ID: "total_sorts", Name: "sorts", Algo: module.Incremental},
			{ID: "sort_overflows", Name: "overflows", Algo: module.Incremental},
		},
	},
	{
		ID:       "row_activity",
		Title:    "Row Activity",
		Units:    "rows/s",
		Fam:      "activity",
		Ctx:      "db2.row_activity",
		Type:     module.Area,
		Priority: prioRowActivity,
		Dims: module.Dims{
			{ID: "rows_read", Name: "read", Algo: module.Incremental},
			{ID: "rows_returned", Name: "returned", Algo: module.Incremental},
			{ID: "rows_modified", Name: "modified", Algo: module.Incremental},
		},
	},
	{
		ID:       "bufferpool_hit_ratio",
		Title:    "Buffer Pool Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_hit_ratio",
		Type:     module.Line,
		Priority: prioBufferpoolHitRatio,
		Dims: module.Dims{
			{ID: "bufferpool_hit_ratio", Name: "hit_ratio", Div: Precision},
		},
	},
	{
		ID:       "bufferpool_detailed_hit_ratio",
		Title:    "Buffer Pool Detailed Hit Ratios",
		Units:    "percentage",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_detailed_hit_ratio",
		Type:     module.Line,
		Priority: prioBufferpoolDetailedHitRatio,
		Dims: module.Dims{
			{ID: "bufferpool_data_hit_ratio", Name: "data", Div: Precision},
			{ID: "bufferpool_index_hit_ratio", Name: "index", Div: Precision},
			{ID: "bufferpool_xda_hit_ratio", Name: "xda", Div: Precision},
			{ID: "bufferpool_column_hit_ratio", Name: "column", Div: Precision},
		},
	},
	{
		ID:       "bufferpool_reads",
		Title:    "Buffer Pool Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolReads,
		Dims: module.Dims{
			{ID: "bufferpool_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	},
	{
		ID:       "bufferpool_data_reads",
		Title:    "Buffer Pool Data Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_data_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolDataReads,
		Dims: module.Dims{
			{ID: "bufferpool_data_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_data_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	},
	{
		ID:       "bufferpool_index_reads",
		Title:    "Buffer Pool Index Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_index_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolIndexReads,
		Dims: module.Dims{
			{ID: "bufferpool_index_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_index_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	},
	{
		ID:       "bufferpool_xda_reads",
		Title:    "Buffer Pool XDA Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_xda_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolXDAReads,
		Dims: module.Dims{
			{ID: "bufferpool_xda_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_xda_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	},
	{
		ID:       "bufferpool_column_reads",
		Title:    "Buffer Pool Column Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_column_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolColumnReads,
		Dims: module.Dims{
			{ID: "bufferpool_column_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_column_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	},
	{
		ID:       "log_space",
		Title:    "Log Space Usage",
		Units:    "bytes",
		Fam:      "logging",
		Ctx:      "db2.log_space",
		Type:     module.Stacked,
		Priority: prioLogSpace,
		Dims: module.Dims{
			{ID: "log_used_space", Name: "used"},
			{ID: "log_available_space", Name: "available"},
		},
	},
	{
		ID:       "log_utilization",
		Title:    "Log Space Utilization",
		Units:    "percentage",
		Fam:      "logging",
		Ctx:      "db2.log_utilization",
		Type:     module.Line,
		Priority: prioLogUtilization,
		Dims: module.Dims{
			{ID: "log_utilization", Name: "utilization"},
		},
	},
	{
		ID:       "log_io",
		Title:    "Log I/O Operations",
		Units:    "operations/s",
		Fam:      "logging",
		Ctx:      "db2.log_io",
		Type:     module.Area,
		Priority: prioLogIO,
		Dims: module.Dims{
			{ID: "log_io_reads", Name: "reads", Algo: module.Incremental},
			{ID: "log_io_writes", Name: "writes", Algo: module.Incremental},
		},
	},
	{
		ID:       "long_running_queries",
		Title:    "Long Running Queries",
		Units:    "queries",
		Fam:      "queries",
		Ctx:      "db2.long_running_queries",
		Type:     module.Stacked,
		Priority: prioLongRunningQueries,
		Dims: module.Dims{
			{ID: "long_running_queries", Name: "total"},
			{ID: "long_running_queries_warning", Name: "warning"},
			{ID: "long_running_queries_critical", Name: "critical"},
		},
	},
	{
		ID:       "backup_status",
		Title:    "Last Backup Status",
		Units:    "status",
		Fam:      "backup",
		Ctx:      "db2.backup_status",
		Type:     module.Line,
		Priority: prioBackupStatus,
		Dims: module.Dims{
			{ID: "last_backup_status", Name: "status"},
		},
	},
	{
		ID:       "backup_age",
		Title:    "Time Since Last Backup",
		Units:    "hours",
		Fam:      "backup",
		Ctx:      "db2.backup_age",
		Type:     module.Line,
		Priority: prioBackupAge,
		Dims: module.Dims{
			{ID: "last_full_backup_age", Name: "full"},
			{ID: "last_incremental_backup_age", Name: "incremental"},
		},
	},
	{
		ID:       "service_health",
		Title:    "Service Health Status",
		Units:    "status",
		Fam:      "health",
		Ctx:      "db2.service_health",
		Type:     module.Line,
		Priority: prioServiceHealth,
		Dims: module.Dims{
			{ID: "can_connect", Name: "connection"},
			{ID: "database_status", Name: "database"},
		},
	},
	{
		ID:       "lock_details",
		Title:    "Lock Details",
		Units:    "locks",
		Fam:      "locking",
		Ctx:      "db2.lock_details",
		Type:     module.Line,
		Priority: prioLockDetails,
		Dims: module.Dims{
			{ID: "lock_active", Name: "active"},
			{ID: "lock_waiting_agents", Name: "waiting_agents"},
			{ID: "lock_memory_pages", Name: "memory_pages"},
		},
	},
	{
		ID:       "lock_wait_time",
		Title:    "Average Lock Wait Time",
		Units:    "milliseconds",
		Fam:      "locking",
		Ctx:      "db2.lock_wait_time",
		Type:     module.Line,
		Priority: prioLockWaitTime,
		Dims: module.Dims{
			{ID: "lock_wait_time", Name: "wait_time"},
		},
	},
}

// Instance chart templates
var (
	databaseStatusChartTmpl = module.Chart{
		ID:       "database_%s_status",
		Title:    "Database Status",
		Units:    "status",
		Fam:      "databases",
		Ctx:      "db2.database_instance_status",
		Type:     module.Line,
		Priority: prioDatabaseStatus,
		Dims: module.Dims{
			{ID: "database_%s_status", Name: "status"},
		},
	}

	databaseApplicationsChartTmpl = module.Chart{
		ID:       "database_%s_applications",
		Title:    "Database Applications",
		Units:    "applications",
		Fam:      "databases",
		Ctx:      "db2.database_applications",
		Type:     module.Line,
		Priority: prioDatabaseApplications,
		Dims: module.Dims{
			{ID: "database_%s_applications", Name: "applications"},
		},
	}

	bufferpoolHitRatioChartTmpl = module.Chart{
		ID:       "bufferpool_%s_hit_ratio",
		Title:    "Buffer Pool Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_hit_ratio",
		Type:     module.Line,
		Priority: prioBufferpoolHitRatioInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_hit_ratio", Name: "overall", Div: Precision},
		},
	}

	bufferpoolDetailedHitRatioChartTmpl = module.Chart{
		ID:       "bufferpool_%s_detailed_hit_ratio",
		Title:    "Buffer Pool Detailed Hit Ratios",
		Units:    "percentage",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_detailed_hit_ratio",
		Type:     module.Line,
		Priority: prioBufferpoolDetailedHitRatioInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_data_hit_ratio", Name: "data", Div: Precision},
			{ID: "bufferpool_%s_index_hit_ratio", Name: "index", Div: Precision},
			{ID: "bufferpool_%s_xda_hit_ratio", Name: "xda", Div: Precision},
			{ID: "bufferpool_%s_column_hit_ratio", Name: "column", Div: Precision},
		},
	}

	bufferpoolReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_reads",
		Title:    "Buffer Pool Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolReadsInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolDataReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_data_reads",
		Title:    "Buffer Pool Data Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_data_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolDataReadsInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_data_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_data_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolIndexReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_index_reads",
		Title:    "Buffer Pool Index Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_index_reads",
		Type:     module.Stacked,
		Priority: prioBufferpoolIndexReadsInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_index_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_index_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolPagesChartTmpl = module.Chart{
		ID:       "bufferpool_%s_pages",
		Title:    "Buffer Pool Pages",
		Units:    "pages",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_pages",
		Type:     module.Stacked,
		Priority: prioBufferpoolPagesInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_used_pages", Name: "used"},
			{ID: "bufferpool_%s_total_pages", Name: "total"},
		},
	}

	bufferpoolWritesChartTmpl = module.Chart{
		ID:       "bufferpool_%s_writes",
		Title:    "Buffer Pool Writes",
		Units:    "writes/s",
		Fam:      "bufferpools/instances",
		Ctx:      "db2.bufferpool_instance_writes",
		Type:     module.Line,
		Priority: prioBufferpoolWritesInstance,
		Dims: module.Dims{
			{ID: "bufferpool_%s_writes", Name: "writes", Algo: module.Incremental},
		},
	}

	tablespaceUsageChartTmpl = module.Chart{
		ID:       "tablespace_%s_usage",
		Title:    "Tablespace Usage",
		Units:    "percentage",
		Fam:      "tablespaces",
		Ctx:      "db2.tablespace_usage",
		Type:     module.Line,
		Priority: prioTablespaceUsage,
		Dims: module.Dims{
			{ID: "tablespace_%s_used_percent", Name: "used"},
		},
	}

	tablespaceSizeChartTmpl = module.Chart{
		ID:       "tablespace_%s_size",
		Title:    "Tablespace Size",
		Units:    "bytes",
		Fam:      "tablespaces",
		Ctx:      "db2.tablespace_size",
		Type:     module.Stacked,
		Priority: prioTablespaceSize,
		Dims: module.Dims{
			{ID: "tablespace_%s_used_size", Name: "used"},
			{ID: "tablespace_%s_free_size", Name: "free"},
		},
	}

	tablespaceUsableSizeChartTmpl = module.Chart{
		ID:       "tablespace_%s_usable_size",
		Title:    "Tablespace Usable Size",
		Units:    "bytes",
		Fam:      "tablespaces",
		Ctx:      "db2.tablespace_usable_size",
		Type:     module.Line,
		Priority: prioTablespaceUsableSize,
		Dims: module.Dims{
			{ID: "tablespace_%s_total_size", Name: "total"},
			{ID: "tablespace_%s_usable_size", Name: "usable"},
		},
	}

	tablespaceStateChartTmpl = module.Chart{
		ID:       "tablespace_%s_state",
		Title:    "Tablespace State",
		Units:    "state",
		Fam:      "tablespaces",
		Ctx:      "db2.tablespace_state",
		Type:     module.Line,
		Priority: prioTablespaceState,
		Dims: module.Dims{
			{ID: "tablespace_%s_state", Name: "state"},
		},
	}

	connectionStateChartTmpl = module.Chart{
		ID:       "connection_%s_state",
		Title:    "Connection State",
		Units:    "state",
		Fam:      "connections/instances",
		Ctx:      "db2.connection_state",
		Type:     module.Line,
		Priority: prioConnectionState,
		Dims: module.Dims{
			{ID: "connection_%s_state", Name: "state"},
		},
	}

	connectionActivityChartTmpl = module.Chart{
		ID:       "connection_%s_activity",
		Title:    "Connection Row Activity",
		Units:    "rows/s",
		Fam:      "connections/instances",
		Ctx:      "db2.connection_activity",
		Type:     module.Area,
		Priority: prioConnectionActivity,
		Dims: module.Dims{
			{ID: "connection_%s_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "connection_%s_rows_written", Name: "written", Algo: module.Incremental},
		},
	}

	connectionWaitTimeChartTmpl = module.Chart{
		ID:       "connection_%s_wait_time",
		Title:    "Connection Wait Time",
		Units:    "milliseconds",
		Fam:      "connections/instances",
		Ctx:      "db2.connection_wait_time",
		Type:     module.Stacked,
		Priority: prioConnectionWaitTime,
		Dims: module.Dims{
			{ID: "connection_%s_lock_wait_time", Name: "lock", Algo: module.Incremental},
			{ID: "connection_%s_log_disk_wait_time", Name: "log_disk", Algo: module.Incremental},
			{ID: "connection_%s_log_buffer_wait_time", Name: "log_buffer", Algo: module.Incremental},
			{ID: "connection_%s_pool_read_time", Name: "pool_read", Algo: module.Incremental},
			{ID: "connection_%s_pool_write_time", Name: "pool_write", Algo: module.Incremental},
			{ID: "connection_%s_direct_read_time", Name: "direct_read", Algo: module.Incremental},
			{ID: "connection_%s_direct_write_time", Name: "direct_write", Algo: module.Incremental},
			{ID: "connection_%s_fcm_recv_wait_time", Name: "fcm_recv", Algo: module.Incremental},
			{ID: "connection_%s_fcm_send_wait_time", Name: "fcm_send", Algo: module.Incremental},
		},
	}

	connectionProcessingTimeChartTmpl = module.Chart{
		ID:       "connection_%s_processing_time",
		Title:    "Connection Processing Time",
		Units:    "milliseconds",
		Fam:      "connections/instances",
		Ctx:      "db2.connection_processing_time",
		Type:     module.Stacked,
		Priority: prioConnectionProcessingTime,
		Dims: module.Dims{
			{ID: "connection_%s_total_routine_time", Name: "routine", Algo: module.Incremental},
			{ID: "connection_%s_total_compile_time", Name: "compile", Algo: module.Incremental},
			{ID: "connection_%s_total_section_time", Name: "section", Algo: module.Incremental},
			{ID: "connection_%s_total_commit_time", Name: "commit", Algo: module.Incremental},
			{ID: "connection_%s_total_rollback_time", Name: "rollback", Algo: module.Incremental},
		},
	}

	tableSizeChartTmpl = module.Chart{
		ID:       "table_%s_size",
		Title:    "Table Size",
		Units:    "bytes",
		Fam:      "tables",
		Ctx:      "db2.table_size",
		Type:     module.Stacked,
		Priority: prioTableSize,
		Dims: module.Dims{
			{ID: "table_%s_data_size", Name: "data"},
			{ID: "table_%s_index_size", Name: "index"},
			{ID: "table_%s_long_obj_size", Name: "long_obj"},
		},
	}

	tableActivityChartTmpl = module.Chart{
		ID:       "table_%s_activity",
		Title:    "Table Activity",
		Units:    "rows/s",
		Fam:      "tables",
		Ctx:      "db2.table_activity",
		Type:     module.Area,
		Priority: prioTableActivity,
		Dims: module.Dims{
			{ID: "table_%s_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "table_%s_rows_written", Name: "written", Algo: module.Incremental},
		},
	}

	indexUsageChartTmpl = module.Chart{
		ID:       "index_%s_usage",
		Title:    "Index Usage",
		Units:    "scans/s",
		Fam:      "indexes",
		Ctx:      "db2.index_usage",
		Type:     module.Area,
		Priority: prioIndexUsage,
		Dims: module.Dims{
			{ID: "index_%s_index_scans", Name: "index", Algo: module.Incremental},
			{ID: "index_%s_full_scans", Name: "full", Algo: module.Incremental},
		},
	}

	memoryPoolUsageChartTmpl = module.Chart{
		ID:       "memory_pool_%s_usage",
		Title:    "Memory Pool Usage",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "db2.memory_pool_usage",
		Type:     module.Line,
		Priority: prioMemoryPoolUsage,
		Dims: module.Dims{
			{ID: "memory_pool_%s_used", Name: "used"},
		},
	}

	memoryPoolHWMChartTmpl = module.Chart{
		ID:       "memory_pool_%s_hwm",
		Title:    "Memory Pool High Water Mark",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "db2.memory_pool_hwm",
		Type:     module.Line,
		Priority: prioMemoryPoolHWM,
		Dims: module.Dims{
			{ID: "memory_pool_%s_hwm", Name: "hwm"},
		},
	}

	tableIOScansChartTmpl = module.Chart{
		ID:       "table_io_%s_scans",
		Title:    "Table Scans",
		Units:    "scans/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_scans",
		Type:     module.Line,
		Priority: prioTableIOScans,
		Dims: module.Dims{
			{ID: "table_io_%s_scans", Name: "scans", Algo: module.Incremental},
		},
	}

	tableIORowsChartTmpl = module.Chart{
		ID:       "table_io_%s_rows",
		Title:    "Table Row Operations",
		Units:    "rows/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_rows",
		Type:     module.Line,
		Priority: prioTableIORows,
		Dims: module.Dims{
			{ID: "table_io_%s_read", Name: "read", Algo: module.Incremental},
		},
	}

	tableIOActivityChartTmpl = module.Chart{
		ID:       "table_io_%s_activity",
		Title:    "Table DML Activity",
		Units:    "operations/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_activity",
		Type:     module.Stacked,
		Priority: prioTableIOActivity,
		Dims: module.Dims{
			{ID: "table_io_%s_inserts", Name: "inserts", Algo: module.Incremental},
			{ID: "table_io_%s_updates", Name: "updates", Algo: module.Incremental},
			{ID: "table_io_%s_deletes", Name: "deletes", Algo: module.Incremental},
		},
	}

	tableIOOverflowChartTmpl = module.Chart{
		ID:       "table_io_%s_overflow",
		Title:    "Table Overflow Accesses",
		Units:    "accesses/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_overflow",
		Type:     module.Line,
		Priority: prioTableIOActivity + 1,
		Dims: module.Dims{
			{ID: "table_io_%s_overflow_accesses", Name: "overflow", Algo: module.Incremental},
		},
	}

	memorySetUsageChartTmpl = module.Chart{
		ID:       "memory_set_%s_usage",
		Title:    "Memory Set Usage",
		Units:    "bytes",
		Fam:      "memory_sets",
		Ctx:      "db2.memory_set_usage",
		Type:     module.Line,
		Priority: prioMemorySetUsage,
		Dims: module.Dims{
			{ID: "memory_set_%s_used", Name: "used"},
		},
	}

	memorySetCommittedChartTmpl = module.Chart{
		ID:       "memory_set_%s_committed",
		Title:    "Memory Set Committed",
		Units:    "bytes",
		Fam:      "memory_sets",
		Ctx:      "db2.memory_set_committed",
		Type:     module.Line,
		Priority: prioMemorySetCommitted,
		Dims: module.Dims{
			{ID: "memory_set_%s_committed", Name: "committed"},
		},
	}

	memorySetHighWaterMarkChartTmpl = module.Chart{
		ID:       "memory_set_%s_high_water_mark",
		Title:    "Memory Set High Water Mark",
		Units:    "bytes",
		Fam:      "memory_sets",
		Ctx:      "db2.memory_set_high_water_mark",
		Type:     module.Line,
		Priority: prioMemorySetHighWaterMark,
		Dims: module.Dims{
			{ID: "memory_set_%s_high_water_mark", Name: "hwm"},
		},
	}

	memorySetAdditionalCommittedChartTmpl = module.Chart{
		ID:       "memory_set_%s_additional_committed",
		Title:    "Memory Set Additional Committed",
		Units:    "bytes",
		Fam:      "memory_sets",
		Ctx:      "db2.memory_set_additional_committed",
		Type:     module.Line,
		Priority: prioMemorySetAdditionalCommitted,
		Dims: module.Dims{
			{ID: "memory_set_%s_additional_committed", Name: "additional"},
		},
	}

	memorySetPercentUsedHWMChartTmpl = module.Chart{
		ID:       "memory_set_%s_percent_used_hwm",
		Title:    "Memory Set Percent Used vs HWM",
		Units:    "percent",
		Fam:      "memory_sets",
		Ctx:      "db2.memory_set_percent_used_hwm",
		Type:     module.Line,
		Priority: prioMemorySetPercentUsedHWM,
		Dims: module.Dims{
			{ID: "memory_set_%s_percent_used_hwm", Name: "used_hwm"},
		},
	}

	prefetcherPrefetchRatioChartTmpl = module.Chart{
		ID:       "prefetcher_%s_prefetch_ratio",
		Title:    "Prefetcher Prefetch Ratio",
		Units:    "percentage",
		Fam:      "prefetchers",
		Ctx:      "db2.prefetcher_prefetch_ratio",
		Type:     module.Line,
		Priority: prioPrefetcherPrefetchRatio,
		Dims: module.Dims{
			{ID: "prefetcher_%s_prefetch_ratio", Name: "ratio"},
		},
	}

	prefetcherCleanerRatioChartTmpl = module.Chart{
		ID:       "prefetcher_%s_cleaner_ratio",
		Title:    "Prefetcher Cleaner Ratio",
		Units:    "percentage",
		Fam:      "prefetchers",
		Ctx:      "db2.prefetcher_cleaner_ratio",
		Type:     module.Line,
		Priority: prioPrefetcherCleanerRatio,
		Dims: module.Dims{
			{ID: "prefetcher_%s_cleaner_ratio", Name: "ratio"},
		},
	}

	prefetcherPhysicalReadsChartTmpl = module.Chart{
		ID:       "prefetcher_%s_physical_reads",
		Title:    "Prefetcher Physical Reads",
		Units:    "reads/s",
		Fam:      "prefetchers",
		Ctx:      "db2.prefetcher_physical_reads",
		Type:     module.Line,
		Priority: prioPrefetcherPhysicalReads,
		Dims: module.Dims{
			{ID: "prefetcher_%s_physical_reads", Name: "reads", Algo: module.Incremental},
		},
	}

	prefetcherAsyncReadsChartTmpl = module.Chart{
		ID:       "prefetcher_%s_async_reads",
		Title:    "Prefetcher Async Reads",
		Units:    "reads/s",
		Fam:      "prefetchers",
		Ctx:      "db2.prefetcher_async_reads",
		Type:     module.Line,
		Priority: prioPrefetcherAsyncReads,
		Dims: module.Dims{
			{ID: "prefetcher_%s_async_reads", Name: "reads", Algo: module.Incremental},
		},
	}

	prefetcherWaitTimeChartTmpl = module.Chart{
		ID:       "prefetcher_%s_wait_time",
		Title:    "Prefetcher Avg Wait Time",
		Units:    "milliseconds",
		Fam:      "prefetchers",
		Ctx:      "db2.prefetcher_wait_time",
		Type:     module.Line,
		Priority: prioPrefetcherWaitTime,
		Dims: module.Dims{
			{ID: "prefetcher_%s_avg_wait_time", Name: "wait_time"},
		},
	}

	prefetcherUnreadPagesChartTmpl = module.Chart{
		ID:       "prefetcher_%s_unread_pages",
		Title:    "Prefetcher Unread Pages",
		Units:    "pages/s",
		Fam:      "prefetchers",
		Ctx:      "db2.prefetcher_unread_pages",
		Type:     module.Line,
		Priority: prioPrefetcherUnreadPages,
		Dims: module.Dims{
			{ID: "prefetcher_%s_unread_pages", Name: "unread", Algo: module.Incremental},
		},
	}
)

// Chart creation functions
func newCharts() *module.Charts {
	return globalCharts.Copy()
}

func (d *DB2) newDatabaseCharts(db *databaseMetrics) *module.Charts {
	charts := module.Charts{
		databaseStatusChartTmpl.Copy(),
		databaseApplicationsChartTmpl.Copy(),
	}

	cleanName := cleanName(db.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "database", Value: db.name},
			{Key: "status", Value: db.status},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newBufferpoolCharts(bp *bufferpoolMetrics) *module.Charts {
	// Get relevant charts based on edition/version
	charts := module.Charts{
		bufferpoolHitRatioChartTmpl.Copy(),
		bufferpoolReadsChartTmpl.Copy(),
		bufferpoolPagesChartTmpl.Copy(),
		bufferpoolWritesChartTmpl.Copy(),
	}

	// Add detailed hit ratio charts if edition supports it
	if !d.isDB2ForAS400 && !d.isDB2ForZOS && !d.isDB2Cloud {
		charts = append(charts,
			bufferpoolDetailedHitRatioChartTmpl.Copy(),
			bufferpoolDataReadsChartTmpl.Copy(),
			bufferpoolIndexReadsChartTmpl.Copy(),
		)
	}

	cleanName := cleanName(bp.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "bufferpool", Value: bp.name},
			{Key: "page_size", Value: fmt.Sprintf("%d", bp.pageSize)},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		
		// Column dimension is now always included since we use MON_GET_BUFFERPOOL
		// The values may be zero in Community Edition
		
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newTablespaceCharts(ts *tablespaceMetrics) *module.Charts {
	charts := module.Charts{
		tablespaceUsageChartTmpl.Copy(),
		tablespaceSizeChartTmpl.Copy(),
		tablespaceUsableSizeChartTmpl.Copy(),
		tablespaceStateChartTmpl.Copy(),
	}

	cleanName := cleanName(ts.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "tablespace", Value: ts.name},
			{Key: "type", Value: ts.tbspType},
			{Key: "content_type", Value: ts.contentType},
			{Key: "state", Value: ts.state},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newConnectionCharts(conn *connectionMetrics) *module.Charts {
	charts := module.Charts{
		connectionStateChartTmpl.Copy(),
		connectionActivityChartTmpl.Copy(),
		connectionWaitTimeChartTmpl.Copy(),
		connectionProcessingTimeChartTmpl.Copy(),
	}

	cleanID := cleanName(conn.applicationID)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanID)
		labels := []module.Label{
			{Key: "application_id", Value: conn.applicationID},
			{Key: "state", Value: conn.connectionState},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}

		// Include optional labels if they have values
		if conn.applicationName != "" && conn.applicationName != "-" {
			labels = append(labels, module.Label{Key: "application_name", Value: conn.applicationName})
		}
		if conn.clientHostname != "" && conn.clientHostname != "-" {
			labels = append(labels, module.Label{Key: "client_hostname", Value: conn.clientHostname})
		}
		if conn.clientIP != "" && conn.clientIP != "-" {
			labels = append(labels, module.Label{Key: "client_ip", Value: conn.clientIP})
		}
		if conn.clientUser != "" && conn.clientUser != "-" {
			labels = append(labels, module.Label{Key: "client_user", Value: conn.clientUser})
		}

		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanID)
		}
	}

	return &charts
}

func (d *DB2) newTableCharts(t *tableMetrics) *module.Charts {
	charts := module.Charts{
		tableSizeChartTmpl.Copy(),
		tableActivityChartTmpl.Copy(),
	}

	cleanName := cleanName(t.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "table", Value: t.name},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newIndexCharts(i *indexMetrics) *module.Charts {
	charts := module.Charts{
		indexUsageChartTmpl.Copy(),
	}

	cleanName := cleanName(i.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "index", Value: i.name},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newMemorySetCharts(ms *memorySetInstanceMetrics) *module.Charts {
	// Create a composite key for the memory set
	setKey := fmt.Sprintf("%s_%s_%s_%d", ms.hostName, ms.dbName, ms.setType, ms.member)
	cleanKey := cleanName(setKey)

	charts := module.Charts{
		memorySetUsageChartTmpl.Copy(),
		memorySetCommittedChartTmpl.Copy(),
		memorySetHighWaterMarkChartTmpl.Copy(),
		memorySetAdditionalCommittedChartTmpl.Copy(),
		memorySetPercentUsedHWMChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanKey)
		chart.Labels = []module.Label{
			{Key: "host", Value: ms.hostName},
			{Key: "database", Value: ms.dbName},
			{Key: "set_type", Value: ms.setType},
			{Key: "member", Value: fmt.Sprintf("%d", ms.member)},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanKey)
		}
	}

	return &charts
}

func (d *DB2) newPrefetcherCharts(p *prefetcherInstanceMetrics, bufferPoolName string) *module.Charts {
	charts := module.Charts{
		prefetcherPrefetchRatioChartTmpl.Copy(),
		prefetcherCleanerRatioChartTmpl.Copy(),
		prefetcherPhysicalReadsChartTmpl.Copy(),
		prefetcherAsyncReadsChartTmpl.Copy(),
		prefetcherWaitTimeChartTmpl.Copy(),
		prefetcherUnreadPagesChartTmpl.Copy(),
	}

	cleanName := cleanName(bufferPoolName)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "buffer_pool", Value: bufferPoolName},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newMemoryPoolCharts(pool *memoryPoolMetrics) *module.Charts {
	charts := module.Charts{
		memoryPoolUsageChartTmpl.Copy(),
		memoryPoolHWMChartTmpl.Copy(),
	}

	cleanName := cleanName(pool.poolType)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "pool_type", Value: pool.poolType},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newTableIOCharts(table *tableMetrics) *module.Charts {
	charts := module.Charts{
		tableIOScansChartTmpl.Copy(),
		tableIORowsChartTmpl.Copy(),
		tableIOActivityChartTmpl.Copy(),
		tableIOOverflowChartTmpl.Copy(),
	}

	cleanName := cleanName(table.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "table", Value: table.name},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}