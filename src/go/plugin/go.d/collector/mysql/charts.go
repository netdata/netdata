// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioBandwidth = module.Priority + iota
	prioQueries
	prioQueriesType
	prioHandlers
	prioTableOpenCacheOverflows
	prioTableLocks
	prioTableJoinIssues
	prioTableSortIssues
	prioTmpOperations
	prioConnections
	prioActiveConnections
	prioBinlogCache
	prioBinlogStatementCache
	prioThreads
	prioThreadsCreated
	prioThreadCacheMisses
	prioInnoDBIO
	prioInnoDBIOOperations
	prioInnoDBIOPendingOperations
	prioInnoDBLog
	prioInnoDBOSLog
	prioInnoDBOSLogFsyncWrites
	prioInnoDBOSLogIO
	prioInnoDBCurRowLock
	prioInnoDBRows
	prioInnoDBBufferPoolPages
	prioInnoDBBufferPoolPagesFlushed
	prioInnoDBBufferPoolBytes
	prioInnoDBBufferPoolReadAhead
	prioInnoDBBufferPoolReadAheadRnd
	prioInnoDBBufferPoolOperations
	prioMyISAMKeyBlocks
	prioMyISAMKeyRequests
	prioMyISAMKeyDiskOperations
	prioOpenFiles
	prioOpenFilesRate
	prioConnectionErrors
	prioOpenedTables
	prioOpenTables
	prioProcessListFetchQueryDuration
	prioProcessListQueries
	prioProcessListLongestQueryDuration
	prioInnoDBDeadlocks
	prioQCacheOperations
	prioQCacheQueries
	prioQCacheFreeMem
	prioQCacheMemBlocks
	prioGaleraWriteSets
	prioGaleraBytes
	prioGaleraQueue
	prioGaleraConflicts
	prioGaleraFlowControl
	prioGaleraClusterStatus
	prioGaleraClusterState
	prioGaleraClusterSize
	prioGaleraClusterWeight
	prioGaleraClusterConnectionStatus
	prioGaleraReadinessState
	prioGaleraOpenTransactions
	prioGaleraThreadCount
	prioSlaveSecondsBehindMaster
	prioSlaveSQLIOThreadRunningState
	prioUserStatsCPUTime
	prioUserStatsRows
	prioUserStatsCommands
	prioUserStatsDeniedCommands
	prioUserStatsTransactions
	prioUserStatsBinlogWritten
	prioUserStatsEmptyQueries
	prioUserStatsConnections
	prioUserStatsLostConnections
	prioUserStatsDeniedConnections
)

var baseCharts = module.Charts{
	chartBandwidth.Copy(),
	chartQueries.Copy(),
	chartQueriesType.Copy(),
	chartHandlers.Copy(),
	chartTableLocks.Copy(),
	chartTableJoinIssues.Copy(),
	chartTableSortIssues.Copy(),
	chartTmpOperations.Copy(),
	chartConnections.Copy(),
	chartActiveConnections.Copy(),
	chartThreads.Copy(),
	chartThreadCreationRate.Copy(),
	chartThreadsCacheMisses.Copy(),
	chartInnoDBIO.Copy(),
	chartInnoDBIOOperations.Copy(),
	chartInnoDBPendingIOOperations.Copy(),
	chartInnoDBLogActivity.Copy(),
	chartInnoDBLogOccupancy.Copy(),
	chartInnoDBLogOperations.Copy(),
	chartInnoDBCheckpointAge.Copy(),
	chartInnoDBCurrentRowLocks.Copy(),
	chartInnoDBRowsOperations.Copy(),
	chartInnoDBBufferPoolPages.Copy(),
	chartInnoDBBufferPoolPagesFlushed.Copy(),
	chartInnoDBBufferPoolBytes.Copy(),
	chartInnoDBBufferPoolReadAhead.Copy(),
	chartInnoDBBufferPoolReadAheadRnd.Copy(),
	chartInnoDBBufferPoolOperations.Copy(),
	chartOpenFiles.Copy(),
	chartOpenedFilesRate.Copy(),
	chartConnectionErrors.Copy(),
	chartOpenedTables.Copy(),
	chartOpenTables.Copy(),
	chartProcessListFetchQueryDuration.Copy(),
	chartProcessListQueries.Copy(),
	chartProcessListLongestQueryDuration.Copy(),
}

var (
	chartBandwidth = module.Chart{
		ID:       "net",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "bandwidth",
		Ctx:      "mysql.net",
		Type:     module.Area,
		Priority: prioBandwidth,
		Dims: module.Dims{
			{ID: "bytes_received", Name: "in", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "bytes_sent", Name: "out", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	}
	chartQueries = module.Chart{
		ID:       "queries",
		Title:    "Queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "mysql.queries",
		Priority: prioQueries,
		Dims: module.Dims{
			{ID: "queries", Name: "queries", Algo: module.Incremental},
			{ID: "questions", Name: "questions", Algo: module.Incremental},
			{ID: "slow_queries", Name: "slow_queries", Algo: module.Incremental},
		},
	}
	chartQueriesType = module.Chart{
		ID:       "queries_type",
		Title:    "Queries By Type",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "mysql.queries_type",
		Type:     module.Stacked,
		Priority: prioQueriesType,
		Dims: module.Dims{
			{ID: "com_select", Name: "select", Algo: module.Incremental},
			{ID: "com_delete", Name: "delete", Algo: module.Incremental},
			{ID: "com_update", Name: "update", Algo: module.Incremental},
			{ID: "com_insert", Name: "insert", Algo: module.Incremental},
			{ID: "com_replace", Name: "replace", Algo: module.Incremental},
		},
	}
	chartHandlers = module.Chart{
		ID:       "handlers",
		Title:    "Handlers",
		Units:    "handlers/s",
		Fam:      "handlers",
		Ctx:      "mysql.handlers",
		Priority: prioHandlers,
		Dims: module.Dims{
			{ID: "handler_commit", Name: "commit", Algo: module.Incremental},
			{ID: "handler_delete", Name: "delete", Algo: module.Incremental},
			{ID: "handler_prepare", Name: "prepare", Algo: module.Incremental},
			{ID: "handler_read_first", Name: "read first", Algo: module.Incremental},
			{ID: "handler_read_key", Name: "read key", Algo: module.Incremental},
			{ID: "handler_read_next", Name: "read next", Algo: module.Incremental},
			{ID: "handler_read_prev", Name: "read prev", Algo: module.Incremental},
			{ID: "handler_read_rnd", Name: "read rnd", Algo: module.Incremental},
			{ID: "handler_read_rnd_next", Name: "read rnd next", Algo: module.Incremental},
			{ID: "handler_rollback", Name: "rollback", Algo: module.Incremental},
			{ID: "handler_savepoint", Name: "savepoint", Algo: module.Incremental},
			{ID: "handler_savepoint_rollback", Name: "savepointrollback", Algo: module.Incremental},
			{ID: "handler_update", Name: "update", Algo: module.Incremental},
			{ID: "handler_write", Name: "write", Algo: module.Incremental},
		},
	}
	chartTableOpenCacheOverflows = module.Chart{
		ID:       "table_open_cache_overflows",
		Title:    "Table open cache overflows",
		Units:    "overflows/s",
		Fam:      "open cache",
		Ctx:      "mysql.table_open_cache_overflows",
		Priority: prioTableOpenCacheOverflows,
		Dims: module.Dims{
			{ID: "table_open_cache_overflows", Name: "open_cache", Algo: module.Incremental},
		},
	}
	chartTableLocks = module.Chart{
		ID:       "table_locks",
		Title:    "Table Locks",
		Units:    "locks/s",
		Fam:      "locks",
		Ctx:      "mysql.table_locks",
		Priority: prioTableLocks,
		Dims: module.Dims{
			{ID: "table_locks_immediate", Name: "immediate", Algo: module.Incremental},
			{ID: "table_locks_waited", Name: "waited", Algo: module.Incremental, Mul: -1},
		},
	}
	chartTableJoinIssues = module.Chart{
		ID:       "join_issues",
		Title:    "Table Select Join Issues",
		Units:    "joins/s",
		Fam:      "issues",
		Ctx:      "mysql.join_issues",
		Priority: prioTableJoinIssues,
		Dims: module.Dims{
			{ID: "select_full_join", Name: "full join", Algo: module.Incremental},
			{ID: "select_full_range_join", Name: "full range join", Algo: module.Incremental},
			{ID: "select_range", Name: "range", Algo: module.Incremental},
			{ID: "select_range_check", Name: "range check", Algo: module.Incremental},
			{ID: "select_scan", Name: "scan", Algo: module.Incremental},
		},
	}
	chartTableSortIssues = module.Chart{
		ID:       "sort_issues",
		Title:    "Table Sort Issues",
		Units:    "issues/s",
		Fam:      "issues",
		Ctx:      "mysql.sort_issues",
		Priority: prioTableSortIssues,
		Dims: module.Dims{
			{ID: "sort_merge_passes", Name: "merge passes", Algo: module.Incremental},
			{ID: "sort_range", Name: "range", Algo: module.Incremental},
			{ID: "sort_scan", Name: "scan", Algo: module.Incremental},
		},
	}
	chartTmpOperations = module.Chart{
		ID:       "tmp",
		Title:    "Tmp Operations",
		Units:    "events/s",
		Fam:      "temporaries",
		Ctx:      "mysql.tmp",
		Priority: prioTmpOperations,
		Dims: module.Dims{
			{ID: "created_tmp_disk_tables", Name: "disk tables", Algo: module.Incremental},
			{ID: "created_tmp_files", Name: "files", Algo: module.Incremental},
			{ID: "created_tmp_tables", Name: "tables", Algo: module.Incremental},
		},
	}
	chartConnections = module.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "mysql.connections",
		Priority: prioConnections,
		Dims: module.Dims{
			{ID: "connections", Name: "all", Algo: module.Incremental},
			{ID: "aborted_connects", Name: "aborted", Algo: module.Incremental},
		},
	}
	chartActiveConnections = module.Chart{
		ID:       "connections_active",
		Title:    "Active Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "mysql.connections_active",
		Priority: prioActiveConnections,
		Dims: module.Dims{
			{ID: "threads_connected", Name: "active"},
			{ID: "max_connections", Name: "limit"},
			{ID: "max_used_connections", Name: "max active"},
		},
	}
	chartThreads = module.Chart{
		ID:       "threads",
		Title:    "Threads",
		Units:    "threads",
		Fam:      "threads",
		Ctx:      "mysql.threads",
		Priority: prioThreads,
		Dims: module.Dims{
			{ID: "threads_connected", Name: "connected"},
			{ID: "threads_cached", Name: "cached", Mul: -1},
			{ID: "threads_running", Name: "running"},
		},
	}
	chartThreadCreationRate = module.Chart{
		ID:       "threads_creation_rate",
		Title:    "Threads Creation Rate",
		Units:    "threads/s",
		Fam:      "threads",
		Ctx:      "mysql.threads_created",
		Priority: prioThreadsCreated,
		Dims: module.Dims{
			{ID: "threads_created", Name: "created", Algo: module.Incremental},
		},
	}
	chartThreadsCacheMisses = module.Chart{
		ID:       "thread_cache_misses",
		Title:    "Threads Cache Misses",
		Units:    "misses",
		Fam:      "threads",
		Ctx:      "mysql.thread_cache_misses",
		Type:     module.Area,
		Priority: prioThreadCacheMisses,
		Dims: module.Dims{
			{ID: "thread_cache_misses", Name: "misses", Div: 100},
		},
	}
	chartInnoDBIO = module.Chart{
		ID:       "innodb_io",
		Title:    "InnoDB I/O Bandwidth",
		Units:    "KiB/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_io",
		Type:     module.Area,
		Priority: prioInnoDBIO,
		Dims: module.Dims{
			{ID: "innodb_data_read", Name: "read", Algo: module.Incremental, Div: 1024},
			{ID: "innodb_data_written", Name: "write", Algo: module.Incremental, Div: 1024},
		},
	}
	chartInnoDBIOOperations = module.Chart{
		ID:       "innodb_io_ops",
		Title:    "InnoDB I/O Operations",
		Units:    "operations/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_io_ops",
		Priority: prioInnoDBIOOperations,
		Dims: module.Dims{
			{ID: "innodb_data_reads", Name: "reads", Algo: module.Incremental},
			{ID: "innodb_data_writes", Name: "writes", Algo: module.Incremental, Mul: -1},
			{ID: "innodb_data_fsyncs", Name: "fsyncs", Algo: module.Incremental},
		},
	}
	chartInnoDBPendingIOOperations = module.Chart{
		ID:       "innodb_io_pending_ops",
		Title:    "InnoDB Pending I/O Operations",
		Units:    "operations",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_io_pending_ops",
		Priority: prioInnoDBIOPendingOperations,
		Dims: module.Dims{
			{ID: "innodb_data_pending_reads", Name: "reads"},
			{ID: "innodb_data_pending_writes", Name: "writes", Mul: -1},
			{ID: "innodb_data_pending_fsyncs", Name: "fsyncs"},
		},
	}
	chartInnoDBLogActivity = module.Chart{
		ID:       "innodb_redo_log_activity",
		Title:    "InnoDB Redo Log Activity",
		Units:    "B/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_redo_log_activity",
		Type:     module.Line,
		Priority: prioInnoDBLog,
		Dims: module.Dims{
			{ID: "innodb_log_sequence_number", Name: "redo_written", Algo: module.Incremental},
			{ID: "innodb_last_checkpoint_at", Name: "checkpointed", Algo: module.Incremental},
		},
	}
	chartInnoDBLogOperations = module.Chart{
		ID:       "innodb_log",
		Title:    "InnoDB Log Operations",
		Units:    "operations/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_log",
		Priority: prioInnoDBLog,
		Dims: module.Dims{
			{ID: "innodb_log_waits", Name: "waits", Algo: module.Incremental},
			{ID: "innodb_log_write_requests", Name: "write requests", Algo: module.Incremental, Mul: -1},
			{ID: "innodb_log_writes", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}
	chartInnoDBLogOccupancy = module.Chart{
		ID:       "innodb_redo_log_occupancy",
		Title:    "InnoDB Redo Log Occupancy",
		Units:    "percentage",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_redo_log_occupancy",
		Type:     module.Area,
		Priority: prioInnoDBLog,
		Dims: module.Dims{
			{ID: "innodb_log_occupancy", Name: "occupancy", Algo: module.Absolute, Div: 1000},
		},
	}
	chartInnoDBCheckpointAge = module.Chart{
		ID:       "innodb_checkpoint_age",
		Title:    "InnoDB Checkpoint Age",
		Units:    "B",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_checkpoint_age",
		Priority: prioInnoDBLog,
		Dims: module.Dims{
			{ID: "innodb_checkpoint_age", Name: "age", Algo: module.Absolute},
		},
	}
	chartInnoDBCurrentRowLocks = module.Chart{
		ID:       "innodb_cur_row_lock",
		Title:    "InnoDB Current Row Locks",
		Units:    "operations",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_cur_row_lock",
		Type:     module.Area,
		Priority: prioInnoDBCurRowLock,
		Dims: module.Dims{
			{ID: "innodb_row_lock_current_waits", Name: "current waits"},
		},
	}
	chartInnoDBRowsOperations = module.Chart{
		ID:       "innodb_rows",
		Title:    "InnoDB Row Operations",
		Units:    "operations/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_rows",
		Type:     module.Area,
		Priority: prioInnoDBRows,
		Dims: module.Dims{
			{ID: "innodb_rows_inserted", Name: "inserted", Algo: module.Incremental},
			{ID: "innodb_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "innodb_rows_updated", Name: "updated", Algo: module.Incremental},
			{ID: "innodb_rows_deleted", Name: "deleted", Algo: module.Incremental, Mul: -1},
		},
	}
	chartInnoDBBufferPoolPages = module.Chart{
		ID:       "innodb_buffer_pool_pages",
		Title:    "InnoDB Buffer Pool Pages",
		Units:    "pages",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_buffer_pool_pages",
		Priority: prioInnoDBBufferPoolPages,
		Dims: module.Dims{
			{ID: "innodb_buffer_pool_pages_data", Name: "data"},
			{ID: "innodb_buffer_pool_pages_dirty", Name: "dirty", Mul: -1},
			{ID: "innodb_buffer_pool_pages_free", Name: "free"},
			{ID: "innodb_buffer_pool_pages_misc", Name: "misc", Mul: -1},
			{ID: "innodb_buffer_pool_pages_total", Name: "total"},
		},
	}
	chartInnoDBBufferPoolPagesFlushed = module.Chart{
		ID:       "innodb_buffer_pool_flush_pages_requests",
		Title:    "InnoDB Buffer Pool Flush Pages Requests",
		Units:    "requests/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_buffer_pool_pages_flushed",
		Priority: prioInnoDBBufferPoolPagesFlushed,
		Dims: module.Dims{
			{ID: "innodb_buffer_pool_pages_flushed", Name: "flush pages", Algo: module.Incremental},
		},
	}
	chartInnoDBBufferPoolBytes = module.Chart{
		ID:       "innodb_buffer_pool_bytes",
		Title:    "InnoDB Buffer Pool Bytes",
		Units:    "MiB",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_buffer_pool_bytes",
		Type:     module.Area,
		Priority: prioInnoDBBufferPoolBytes,
		Dims: module.Dims{
			{ID: "innodb_buffer_pool_bytes_data", Name: "data", Div: 1024 * 1024},
			{ID: "innodb_buffer_pool_bytes_dirty", Name: "dirty", Mul: -1, Div: 1024 * 1024},
		},
	}
	chartInnoDBBufferPoolReadAhead = module.Chart{
		ID:       "innodb_buffer_pool_read_ahead",
		Title:    "InnoDB Buffer Pool Read Pages",
		Units:    "pages/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_buffer_pool_read_ahead",
		Type:     module.Area,
		Priority: prioInnoDBBufferPoolReadAhead,
		Dims: module.Dims{
			{ID: "innodb_buffer_pool_read_ahead", Name: "all", Algo: module.Incremental},
			{ID: "innodb_buffer_pool_read_ahead_evicted", Name: "evicted", Algo: module.Incremental, Mul: -1},
		},
	}
	chartInnoDBBufferPoolReadAheadRnd = module.Chart{
		ID:       "innodb_buffer_pool_read_ahead_rnd",
		Title:    "InnoDB Buffer Pool Random Read-Aheads",
		Units:    "operations/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_buffer_pool_read_ahead_rnd",
		Priority: prioInnoDBBufferPoolReadAheadRnd,
		Dims: module.Dims{
			{ID: "innodb_buffer_pool_read_ahead_rnd", Name: "read-ahead", Algo: module.Incremental},
		},
	}
	chartInnoDBBufferPoolOperations = module.Chart{
		ID:       "innodb_buffer_pool_ops",
		Title:    "InnoDB Buffer Pool Operations",
		Units:    "operations/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_buffer_pool_ops",
		Type:     module.Area,
		Priority: prioInnoDBBufferPoolOperations,
		Dims: module.Dims{
			{ID: "innodb_buffer_pool_reads", Name: "disk reads", Algo: module.Incremental},
			{ID: "innodb_buffer_pool_wait_free", Name: "wait free", Algo: module.Incremental, Mul: -1, Div: 1},
		},
	}
	chartOpenFiles = module.Chart{
		ID:       "files",
		Title:    "Open Files",
		Units:    "files",
		Fam:      "files",
		Ctx:      "mysql.files",
		Priority: prioOpenFiles,
		Dims: module.Dims{
			{ID: "open_files", Name: "files"},
		},
	}
	chartOpenedFilesRate = module.Chart{
		ID:       "files_rate",
		Title:    "Opened Files Rate",
		Units:    "files/s",
		Fam:      "files",
		Ctx:      "mysql.files_rate",
		Priority: prioOpenFilesRate,
		Dims: module.Dims{
			{ID: "opened_files", Name: "files", Algo: module.Incremental},
		},
	}
	chartConnectionErrors = module.Chart{
		ID:       "connection_errors",
		Title:    "Connection Errors",
		Units:    "errors/s",
		Fam:      "connections",
		Ctx:      "mysql.connection_errors",
		Priority: prioConnectionErrors,
		Dims: module.Dims{
			{ID: "connection_errors_accept", Name: "accept", Algo: module.Incremental},
			{ID: "connection_errors_internal", Name: "internal", Algo: module.Incremental},
			{ID: "connection_errors_max_connections", Name: "max", Algo: module.Incremental},
			{ID: "connection_errors_peer_address", Name: "peer addr", Algo: module.Incremental},
			{ID: "connection_errors_select", Name: "select", Algo: module.Incremental},
			{ID: "connection_errors_tcpwrap", Name: "tcpwrap", Algo: module.Incremental},
		},
	}
	chartOpenedTables = module.Chart{
		ID:       "opened_tables",
		Title:    "Opened Tables",
		Units:    "tables/s",
		Fam:      "open tables",
		Ctx:      "mysql.opened_tables",
		Priority: prioOpenedTables,
		Dims: module.Dims{
			{ID: "opened_tables", Name: "tables", Algo: module.Incremental},
		},
	}
	chartOpenTables = module.Chart{
		ID:       "open_tables",
		Title:    "Open Tables",
		Units:    "tables",
		Fam:      "open tables",
		Ctx:      "mysql.open_tables",
		Type:     module.Area,
		Priority: prioOpenTables,
		Dims: module.Dims{
			{ID: "table_open_cache", Name: "cache"},
			{ID: "open_tables", Name: "tables"},
		},
	}
	chartProcessListFetchQueryDuration = module.Chart{
		ID:       "process_list_fetch_duration",
		Title:    "Process List Fetch Duration",
		Units:    "milliseconds",
		Fam:      "process list",
		Ctx:      "mysql.process_list_fetch_query_duration",
		Priority: prioProcessListFetchQueryDuration,
		Dims: module.Dims{
			{ID: "process_list_fetch_query_duration", Name: "duration"},
		},
	}
	chartProcessListQueries = module.Chart{
		ID:       "process_list_queries_count",
		Title:    "Queries Count",
		Units:    "queries",
		Fam:      "process list",
		Ctx:      "mysql.process_list_queries_count",
		Type:     module.Stacked,
		Priority: prioProcessListQueries,
		Dims: module.Dims{
			{ID: "process_list_queries_count_system", Name: "system"},
			{ID: "process_list_queries_count_user", Name: "user"},
		},
	}
	chartProcessListLongestQueryDuration = module.Chart{
		ID:       "process_list_longest_query_duration",
		Title:    "Longest Query Duration",
		Units:    "seconds",
		Fam:      "process list",
		Ctx:      "mysql.process_list_longest_query_duration",
		Priority: prioProcessListLongestQueryDuration,
		Dims: module.Dims{
			{ID: "process_list_longest_query_duration", Name: "duration"},
		},
	}
)

var chartsInnoDBOSLog = module.Charts{
	chartInnoDBOSLogPendingOperations.Copy(),
	chartInnoDBOSLogOperations.Copy(),
	chartInnoDBOSLogIO.Copy(),
}

var (
	chartInnoDBOSLogPendingOperations = module.Chart{
		ID:       "innodb_os_log",
		Title:    "InnoDB OS Log Pending Operations",
		Units:    "operations",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_os_log",
		Priority: prioInnoDBOSLog,
		Dims: module.Dims{
			{ID: "innodb_os_log_pending_fsyncs", Name: "fsyncs"},
			{ID: "innodb_os_log_pending_writes", Name: "writes", Mul: -1},
		},
	}
	chartInnoDBOSLogOperations = module.Chart{
		ID:       "innodb_os_log_fsync_writes",
		Title:    "InnoDB OS Log Operations",
		Units:    "operations/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_os_log_fsync_writes",
		Priority: prioInnoDBOSLogFsyncWrites,
		Dims: module.Dims{
			{ID: "innodb_os_log_fsyncs", Name: "fsyncs", Algo: module.Incremental},
		},
	}
	chartInnoDBOSLogIO = module.Chart{
		ID:       "innodb_os_log_io",
		Title:    "InnoDB OS Log Bandwidth",
		Units:    "KiB/s",
		Fam:      "innodb",
		Ctx:      "mysql.innodb_os_log_io",
		Type:     module.Area,
		Priority: prioInnoDBOSLogIO,
		Dims: module.Dims{
			{ID: "innodb_os_log_written", Name: "write", Algo: module.Incremental, Mul: -1, Div: 1024},
		},
	}
)

var chartInnoDBDeadlocks = module.Chart{
	ID:       "innodb_deadlocks",
	Title:    "InnoDB Deadlocks",
	Units:    "operations/s",
	Fam:      "innodb",
	Ctx:      "mysql.innodb_deadlocks",
	Type:     module.Area,
	Priority: prioInnoDBDeadlocks,
	Dims: module.Dims{
		{ID: "innodb_deadlocks", Name: "deadlocks", Algo: module.Incremental},
	},
}

var chartsQCache = module.Charts{
	chartQCacheOperations.Copy(),
	chartQCacheQueries.Copy(),
	chartQCacheFreeMemory.Copy(),
	chartQCacheMemoryBlocks.Copy(),
}

var (
	chartQCacheOperations = module.Chart{
		ID:       "qcache_ops",
		Title:    "QCache Operations",
		Units:    "queries/s",
		Fam:      "qcache",
		Ctx:      "mysql.qcache_ops",
		Priority: prioQCacheOperations,
		Dims: module.Dims{
			{ID: "qcache_hits", Name: "hits", Algo: module.Incremental},
			{ID: "qcache_lowmem_prunes", Name: "lowmem prunes", Algo: module.Incremental, Mul: -1},
			{ID: "qcache_inserts", Name: "inserts", Algo: module.Incremental},
			{ID: "qcache_not_cached", Name: "not cached", Algo: module.Incremental, Mul: -1},
		},
	}
	chartQCacheQueries = module.Chart{
		ID:       "qcache",
		Title:    "QCache Queries in Cache",
		Units:    "queries",
		Fam:      "qcache",
		Ctx:      "mysql.qcache",
		Priority: prioQCacheQueries,
		Dims: module.Dims{
			{ID: "qcache_queries_in_cache", Name: "queries", Algo: module.Absolute},
		},
	}
	chartQCacheFreeMemory = module.Chart{
		ID:       "qcache_freemem",
		Title:    "QCache Free Memory",
		Units:    "MiB",
		Fam:      "qcache",
		Ctx:      "mysql.qcache_freemem",
		Type:     module.Area,
		Priority: prioQCacheFreeMem,
		Dims: module.Dims{
			{ID: "qcache_free_memory", Name: "free", Div: 1024 * 1024},
		},
	}
	chartQCacheMemoryBlocks = module.Chart{
		ID:       "qcache_memblocks",
		Title:    "QCache Memory Blocks",
		Units:    "blocks",
		Fam:      "qcache",
		Ctx:      "mysql.qcache_memblocks",
		Priority: prioQCacheMemBlocks,
		Dims: module.Dims{
			{ID: "qcache_free_blocks", Name: "free"},
			{ID: "qcache_total_blocks", Name: "total"},
		},
	}
)

var chartsGalera = module.Charts{
	chartGaleraWriteSets.Copy(),
	chartGaleraBytes.Copy(),
	chartGaleraQueue.Copy(),
	chartGaleraConflicts.Copy(),
	chartGaleraFlowControl.Copy(),
	chartGaleraClusterStatus.Copy(),
	chartGaleraClusterState.Copy(),
	chartGaleraClusterSize.Copy(),
	chartGaleraClusterWeight.Copy(),
	chartGaleraClusterConnectionStatus.Copy(),
	chartGaleraReadinessState.Copy(),
	chartGaleraOpenTransactions.Copy(),
	chartGaleraThreads.Copy(),
}
var (
	chartGaleraWriteSets = module.Chart{
		ID:       "galera_writesets",
		Title:    "Replicated Writesets",
		Units:    "writesets/s",
		Fam:      "galera",
		Ctx:      "mysql.galera_writesets",
		Priority: prioGaleraWriteSets,
		Dims: module.Dims{
			{ID: "wsrep_received", Name: "rx", Algo: module.Incremental},
			{ID: "wsrep_replicated", Name: "tx", Algo: module.Incremental, Mul: -1},
		},
	}
	chartGaleraBytes = module.Chart{
		ID:       "galera_bytes",
		Title:    "Replicated Bytes",
		Units:    "KiB/s",
		Fam:      "galera",
		Ctx:      "mysql.galera_bytes",
		Type:     module.Area,
		Priority: prioGaleraBytes,
		Dims: module.Dims{
			{ID: "wsrep_received_bytes", Name: "rx", Algo: module.Incremental, Div: 1024},
			{ID: "wsrep_replicated_bytes", Name: "tx", Algo: module.Incremental, Mul: -1, Div: 1024},
		},
	}
	chartGaleraQueue = module.Chart{
		ID:       "galera_queue",
		Title:    "Galera Queue",
		Units:    "writesets",
		Fam:      "galera",
		Ctx:      "mysql.galera_queue",
		Priority: prioGaleraQueue,
		Dims: module.Dims{
			{ID: "wsrep_local_recv_queue", Name: "rx"},
			{ID: "wsrep_local_send_queue", Name: "tx", Mul: -1},
		},
	}
	chartGaleraConflicts = module.Chart{
		ID:       "galera_conflicts",
		Title:    "Replication Conflicts",
		Units:    "transactions",
		Fam:      "galera",
		Ctx:      "mysql.galera_conflicts",
		Type:     module.Area,
		Priority: prioGaleraConflicts,
		Dims: module.Dims{
			{ID: "wsrep_local_bf_aborts", Name: "bf aborts", Algo: module.Incremental},
			{ID: "wsrep_local_cert_failures", Name: "cert fails", Algo: module.Incremental, Mul: -1},
		},
	}
	chartGaleraFlowControl = module.Chart{
		ID:       "galera_flow_control",
		Title:    "Flow Control",
		Units:    "ms",
		Fam:      "galera",
		Ctx:      "mysql.galera_flow_control",
		Type:     module.Area,
		Priority: prioGaleraFlowControl,
		Dims: module.Dims{
			{ID: "wsrep_flow_control_paused_ns", Name: "paused", Algo: module.Incremental, Div: 1000000},
		},
	}
	chartGaleraClusterStatus = module.Chart{
		ID:       "galera_cluster_status",
		Title:    "Cluster Component Status",
		Units:    "status",
		Fam:      "galera",
		Ctx:      "mysql.galera_cluster_status",
		Priority: prioGaleraClusterStatus,
		Dims: module.Dims{
			{ID: "wsrep_cluster_status_primary", Name: "primary"},
			{ID: "wsrep_cluster_status_non_primary", Name: "non_primary"},
			{ID: "wsrep_cluster_status_disconnected", Name: "disconnected"},
		},
	}
	chartGaleraClusterState = module.Chart{
		ID:       "galera_cluster_state",
		Title:    "Cluster Component State",
		Units:    "state",
		Fam:      "galera",
		Ctx:      "mysql.galera_cluster_state",
		Priority: prioGaleraClusterState,
		Dims: module.Dims{
			{ID: "wsrep_local_state_undefined", Name: "undefined"},
			{ID: "wsrep_local_state_joiner", Name: "joining"},
			{ID: "wsrep_local_state_donor", Name: "donor"},
			{ID: "wsrep_local_state_joined", Name: "joined"},
			{ID: "wsrep_local_state_synced", Name: "synced"},
			{ID: "wsrep_local_state_error", Name: "error"},
		},
	}
	chartGaleraClusterSize = module.Chart{
		ID:       "galera_cluster_size",
		Title:    "Number of Nodes in the Cluster",
		Units:    "nodes",
		Fam:      "galera",
		Ctx:      "mysql.galera_cluster_size",
		Priority: prioGaleraClusterSize,
		Dims: module.Dims{
			{ID: "wsrep_cluster_size", Name: "nodes"},
		},
	}
	chartGaleraClusterWeight = module.Chart{
		ID:       "galera_cluster_weight",
		Title:    "The Total Weight of the Current Members in the Cluster",
		Units:    "weight",
		Fam:      "galera",
		Ctx:      "mysql.galera_cluster_weight",
		Priority: prioGaleraClusterWeight,
		Dims: module.Dims{
			{ID: "wsrep_cluster_weight", Name: "weight"},
		},
	}
	chartGaleraClusterConnectionStatus = module.Chart{
		ID:       "galera_connected",
		Title:    "Cluster Connection Status",
		Units:    "boolean",
		Fam:      "galera",
		Ctx:      "mysql.galera_connected",
		Priority: prioGaleraClusterConnectionStatus,
		Dims: module.Dims{
			{ID: "wsrep_connected", Name: "connected"},
		},
	}
	chartGaleraReadinessState = module.Chart{
		ID:       "galera_ready",
		Title:    "Accept Queries Readiness Status",
		Units:    "boolean",
		Fam:      "galera",
		Ctx:      "mysql.galera_ready",
		Priority: prioGaleraReadinessState,
		Dims: module.Dims{
			{ID: "wsrep_ready", Name: "ready"},
		},
	}
	chartGaleraOpenTransactions = module.Chart{
		ID:       "galera_open_transactions",
		Title:    "Open Transactions",
		Units:    "transactions",
		Fam:      "galera",
		Ctx:      "mysql.galera_open_transactions",
		Priority: prioGaleraOpenTransactions,
		Dims: module.Dims{
			{ID: "wsrep_open_transactions", Name: "open"},
		},
	}
	chartGaleraThreads = module.Chart{
		ID:       "galera_thread_count",
		Title:    "Total Number of WSRep (applier/rollbacker) Threads",
		Units:    "threads",
		Fam:      "galera",
		Ctx:      "mysql.galera_thread_count",
		Priority: prioGaleraThreadCount,
		Dims: module.Dims{
			{ID: "wsrep_thread_count", Name: "threads"},
		},
	}
)

var chartsMyISAM = module.Charts{
	chartMyISAMKeyCacheBlocks.Copy(),
	chartMyISAMKeyCacheRequests.Copy(),
	chartMyISAMKeyCacheDiskOperations.Copy(),
}
var (
	chartMyISAMKeyCacheBlocks = module.Chart{
		ID:       "key_blocks",
		Title:    "MyISAM Key Cache Blocks",
		Units:    "blocks",
		Fam:      "myisam",
		Ctx:      "mysql.key_blocks",
		Priority: prioMyISAMKeyBlocks,
		Dims: module.Dims{
			{ID: "key_blocks_unused", Name: "unused"},
			{ID: "key_blocks_used", Name: "used", Mul: -1},
			{ID: "key_blocks_not_flushed", Name: "not flushed"},
		},
	}
	chartMyISAMKeyCacheRequests = module.Chart{
		ID:       "key_requests",
		Title:    "MyISAM Key Cache Requests",
		Units:    "requests/s",
		Fam:      "myisam",
		Ctx:      "mysql.key_requests",
		Type:     module.Area,
		Priority: prioMyISAMKeyRequests,
		Dims: module.Dims{
			{ID: "key_read_requests", Name: "reads", Algo: module.Incremental},
			{ID: "key_write_requests", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}
	chartMyISAMKeyCacheDiskOperations = module.Chart{
		ID:       "key_disk_ops",
		Title:    "MyISAM Key Cache Disk Operations",
		Units:    "operations/s",
		Fam:      "myisam",
		Ctx:      "mysql.key_disk_ops",
		Priority: prioMyISAMKeyDiskOperations,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "key_reads", Name: "reads", Algo: module.Incremental},
			{ID: "key_writes", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}
)

var chartsBinlog = module.Charts{
	chartBinlogCache.Copy(),
	chartBinlogStatementCache.Copy(),
}

var (
	chartBinlogCache = module.Chart{
		ID:       "binlog_cache",
		Title:    "Binlog Cache",
		Units:    "transactions/s",
		Fam:      "binlog",
		Ctx:      "mysql.binlog_cache",
		Priority: prioBinlogCache,
		Dims: module.Dims{
			{ID: "binlog_cache_disk_use", Name: "disk", Algo: module.Incremental},
			{ID: "binlog_cache_use", Name: "all", Algo: module.Incremental},
		},
	}
	chartBinlogStatementCache = module.Chart{
		ID:       "binlog_stmt_cache",
		Title:    "Binlog Statement Cache",
		Units:    "statements/s",
		Fam:      "binlog",
		Ctx:      "mysql.binlog_stmt_cache",
		Priority: prioBinlogStatementCache,
		Dims: module.Dims{
			{ID: "binlog_stmt_cache_disk_use", Name: "disk", Algo: module.Incremental},
			{ID: "binlog_stmt_cache_use", Name: "all", Algo: module.Incremental},
		},
	}
)

var (
	chartsSlaveReplication = module.Charts{
		chartSlaveBehindSeconds.Copy(),
		chartSlaveSQLIOThreadRunningState.Copy(),
	}

	chartSlaveBehindSeconds = module.Chart{
		ID:       "slave_behind",
		Title:    "Slave Behind Seconds",
		Units:    "seconds",
		Fam:      "slave",
		Ctx:      "mysql.slave_behind",
		Priority: prioSlaveSecondsBehindMaster,
		Dims: module.Dims{
			{ID: "seconds_behind_master", Name: "seconds"},
		},
	}
	chartSlaveSQLIOThreadRunningState = module.Chart{
		ID:       "slave_thread_running",
		Title:    "I/O / SQL Thread Running State",
		Units:    "boolean",
		Fam:      "slave",
		Ctx:      "mysql.slave_status",
		Priority: prioSlaveSQLIOThreadRunningState,
		Dims: module.Dims{
			{ID: "slave_sql_running", Name: "sql_running"},
			{ID: "slave_io_running", Name: "io_running"},
		},
	}
)

func newSlaveReplConnCharts(conn string) *module.Charts {
	orig := conn
	conn = strings.ToLower(conn)
	cs := chartsSlaveReplication.Copy()
	for _, chart := range *cs {
		chart.ID += "_" + conn
		chart.Title += " Connection " + orig
		for _, dim := range chart.Dims {
			dim.ID += "_" + conn
		}
	}
	return cs
}

func newMariaDBUserStatisticsCharts(user string) *module.Charts {
	lcUser := strings.ToLower(user)
	charts := chartsTmplUserStats.Copy()
	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, lcUser)
		c.Labels = []module.Label{
			{Key: "user", Value: user},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, lcUser)
		}
	}
	return charts
}

func newPerconaUserStatisticsCharts(user string) *module.Charts {
	lcUser := strings.ToLower(user)
	charts := chartsTmplPerconaUserStats.Copy()
	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, lcUser)
		c.Labels = []module.Label{
			{Key: "user", Value: user},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, lcUser)
		}
	}
	return charts
}

var (
	chartsTmplUserStats = module.Charts{
		chartUserStatsCPU.Copy(),
		chartTmplUserStatsRowsOperations.Copy(),
		chartTmplUserStatsCommands.Copy(),
		chartTmplUserStatsDeniedCommands.Copy(),
		chartTmplUserStatsTransactions.Copy(),
		chartTmplUserStatsBinlogWritten.Copy(),
		chartTmplUserStatsEmptyQueries.Copy(),
		chartTmplUserStatsCreatedConnections.Copy(),
		chartTmplUserStatsLostConnections.Copy(),
		chartTmplUserStatsDeniedConnections.Copy(),
	}
	chartsTmplPerconaUserStats = module.Charts{
		chartUserStatsCPU.Copy(),
		chartTmplPerconaUserStatsRowsOperations.Copy(),
		chartTmplUserStatsCommands.Copy(),
		chartTmplUserStatsDeniedCommands.Copy(),
		chartTmplUserStatsTransactions.Copy(),
		chartTmplUserStatsBinlogWritten.Copy(),
		chartTmplUserStatsEmptyQueries.Copy(),
		chartTmplUserStatsCreatedConnections.Copy(),
		chartTmplUserStatsLostConnections.Copy(),
		chartTmplUserStatsDeniedConnections.Copy(),
	}

	chartUserStatsCPU = module.Chart{
		ID:       "userstats_cpu_%s",
		Title:    "User CPU Time",
		Units:    "percentage",
		Fam:      "user cpu time",
		Ctx:      "mysql.userstats_cpu",
		Priority: prioUserStatsCPUTime,
		Dims: module.Dims{
			{ID: "userstats_%s_cpu_time", Name: "used", Mul: 100, Div: 1000, Algo: module.Incremental},
		},
	}
	chartTmplUserStatsRowsOperations = module.Chart{
		ID:       "userstats_rows_%s",
		Title:    "User Rows Operations",
		Units:    "operations/s",
		Fam:      "user operations",
		Ctx:      "mysql.userstats_rows",
		Type:     module.Stacked,
		Priority: prioUserStatsRows,
		Dims: module.Dims{
			{ID: "userstats_%s_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "userstats_%s_rows_sent", Name: "sent", Algo: module.Incremental},
			{ID: "userstats_%s_rows_updated", Name: "updated", Algo: module.Incremental},
			{ID: "userstats_%s_rows_inserted", Name: "inserted", Algo: module.Incremental},
			{ID: "userstats_%s_rows_deleted", Name: "deleted", Algo: module.Incremental},
		},
	}
	chartTmplPerconaUserStatsRowsOperations = module.Chart{
		ID:       "userstats_rows_%s",
		Title:    "User Rows Operations",
		Units:    "operations/s",
		Fam:      "user operations",
		Ctx:      "mysql.userstats_rows",
		Type:     module.Stacked,
		Priority: prioUserStatsRows,
		Dims: module.Dims{
			{ID: "userstats_%s_rows_fetched", Name: "fetched", Algo: module.Incremental},
			{ID: "userstats_%s_rows_updated", Name: "updated", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsCommands = module.Chart{
		ID:       "userstats_commands_%s",
		Title:    "User Commands",
		Units:    "commands/s",
		Fam:      "user commands",
		Ctx:      "mysql.userstats_commands",
		Type:     module.Stacked,
		Priority: prioUserStatsCommands,
		Dims: module.Dims{
			{ID: "userstats_%s_select_commands", Name: "select", Algo: module.Incremental},
			{ID: "userstats_%s_update_commands", Name: "update", Algo: module.Incremental},
			{ID: "userstats_%s_other_commands", Name: "other", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsDeniedCommands = module.Chart{
		ID:       "userstats_denied_commands_%s",
		Title:    "User Denied Commands",
		Units:    "commands/s",
		Fam:      "user commands denied",
		Ctx:      "mysql.userstats_denied_commands",
		Priority: prioUserStatsDeniedCommands,
		Dims: module.Dims{
			{ID: "userstats_%s_access_denied", Name: "denied", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsTransactions = module.Chart{
		ID:       "userstats_transactions_%s",
		Title:    "User Transactions",
		Units:    "transactions/s",
		Fam:      "user transactions",
		Ctx:      "mysql.userstats_created_transactions",
		Type:     module.Area,
		Priority: prioUserStatsTransactions,
		Dims: module.Dims{
			{ID: "userstats_%s_commit_transactions", Name: "commit", Algo: module.Incremental},
			{ID: "userstats_%s_rollback_transactions", Name: "rollback", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsBinlogWritten = module.Chart{
		ID:       "userstats_binlog_written_%s",
		Title:    "User Binlog Written",
		Units:    "B/s",
		Fam:      "user binlog written",
		Ctx:      "mysql.userstats_binlog_written",
		Priority: prioUserStatsBinlogWritten,
		Dims: module.Dims{
			{ID: "userstats_%s_binlog_bytes_written", Name: "written", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsEmptyQueries = module.Chart{
		ID:       "userstats_empty_queries_%s",
		Title:    "User Empty Queries",
		Units:    "queries/s",
		Fam:      "user empty queries",
		Ctx:      "mysql.userstats_empty_queries",
		Priority: prioUserStatsEmptyQueries,
		Dims: module.Dims{
			{ID: "userstats_%s_empty_queries", Name: "empty", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsCreatedConnections = module.Chart{
		ID:       "userstats_connections_%s",
		Title:    "User Created Connections",
		Units:    "connections/s",
		Fam:      "user connections created ",
		Ctx:      "mysql.userstats_connections",
		Priority: prioUserStatsConnections,
		Dims: module.Dims{
			{ID: "userstats_%s_total_connections", Name: "created", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsLostConnections = module.Chart{
		ID:       "userstats_lost_connections_%s",
		Title:    "User Lost Connections",
		Units:    "connections/s",
		Fam:      "user connections lost",
		Ctx:      "mysql.userstats_lost_connections",
		Priority: prioUserStatsLostConnections,
		Dims: module.Dims{
			{ID: "userstats_%s_lost_connections", Name: "lost", Algo: module.Incremental},
		},
	}
	chartTmplUserStatsDeniedConnections = module.Chart{
		ID:       "userstats_denied_connections_%s",
		Title:    "User Denied Connections",
		Units:    "connections/s",
		Fam:      "user connections denied",
		Ctx:      "mysql.userstats_denied_connections",
		Priority: prioUserStatsDeniedConnections,
		Dims: module.Dims{
			{ID: "userstats_%s_denied_connections", Name: "denied", Algo: module.Incremental},
		},
	}
)

func (c *Collector) addSlaveReplicationConnCharts(conn string) {
	var charts *module.Charts
	if conn == "" {
		charts = chartsSlaveReplication.Copy()
	} else {
		charts = newSlaveReplConnCharts(conn)
	}
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addUserStatisticsCharts(user string) {
	if c.isPercona {
		if err := c.Charts().Add(*newPerconaUserStatisticsCharts(user)...); err != nil {
			c.Warning(err)
		}
	} else {
		if err := c.Charts().Add(*newMariaDBUserStatisticsCharts(user)...); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addInnoDBOSLogCharts() {
	if err := c.Charts().Add(*chartsInnoDBOSLog.Copy()...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addInnoDBOSLogIOChart() {
	if err := c.Charts().Add(chartInnoDBOSLogIO.Copy()); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addMyISAMCharts() {
	if err := c.Charts().Add(*chartsMyISAM.Copy()...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addBinlogCharts() {
	if err := c.Charts().Add(*chartsBinlog.Copy()...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addInnodbDeadlocksChart() {
	if err := c.Charts().Add(chartInnoDBDeadlocks.Copy()); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addQCacheCharts() {
	if err := c.Charts().Add(*chartsQCache.Copy()...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addGaleraCharts() {
	if err := c.Charts().Add(*chartsGalera.Copy()...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addTableOpenCacheOverflowChart() {
	if err := c.Charts().Add(chartTableOpenCacheOverflows.Copy()); err != nil {
		c.Warning(err)
	}
}
