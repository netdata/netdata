// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package db2

type metricsData struct {
	// Database Overview metrics (Screen 01)
	DatabaseStatusActive    int64 `stm:"database_status_active"`
	DatabaseStatusInactive  int64 `stm:"database_status_inactive"`
	DatabaseCountActive     int64 `stm:"database_count_active"`
	DatabaseCountInactive   int64 `stm:"database_count_inactive"`
	CPUUser                 int64 `stm:"cpu_user"`
	CPUSystem               int64 `stm:"cpu_system"`
	CPUIdle                 int64 `stm:"cpu_idle"`
	CPUIowait               int64 `stm:"cpu_iowait"`
	ConnectionsActive       int64 `stm:"connections_active"`
	ConnectionsTotal        int64 `stm:"connections_total"`
	MemoryDatabaseCommitted int64 `stm:"memory_database_committed"`
	MemoryInstanceCommitted int64 `stm:"memory_instance_committed"`
	MemoryBufferpoolUsed    int64 `stm:"memory_bufferpool_used"`
	MemorySharedSortUsed    int64 `stm:"memory_shared_sort_used"`
	OpsSelectStmts          int64 `stm:"ops_select_stmts"`
	OpsUIDStmts             int64 `stm:"ops_uid_stmts"`
	OpsTransactions         int64 `stm:"ops_transactions"`
	OpsActivitiesAborted    int64 `stm:"ops_activities_aborted"`
	TimeAvgDirectRead       int64 `stm:"time_avg_direct_read"`
	TimeAvgDirectWrite      int64 `stm:"time_avg_direct_write"`
	TimeAvgPoolRead         int64 `stm:"time_avg_pool_read"`
	TimeAvgPoolWrite        int64 `stm:"time_avg_pool_write"`

	// Enhanced logging metrics (Screen 18)
	LogCommits          int64 `stm:"log_commits"`
	LogRollbacks        int64 `stm:"log_rollbacks"`
	LogBufferFullEvents int64 `stm:"log_buffer_full_events"`
	LogAvgCommitTime    int64 `stm:"log_avg_commit_time"`
	LogAvgReadTime      int64 `stm:"log_avg_read_time"`
	LogAvgWriteTime     int64 `stm:"log_avg_write_time"`

	// Federation metrics (Screen 32)
	FedConnectionsActive int64 `stm:"fed_connections_active"`
	FedConnectionsIdle   int64 `stm:"fed_connections_idle"`
	FedRowsRead          int64 `stm:"fed_rows_read"`
	FedSelectStmts       int64 `stm:"fed_select_stmts"`
	FedWaitsTotal        int64 `stm:"fed_waits_total"`

	// Connection metrics
	ConnTotal     int64 `stm:"conn_total"`
	ConnActive    int64 `stm:"conn_active"`
	ConnExecuting int64 `stm:"conn_executing"`
	ConnIdle      int64 `stm:"conn_idle"`
	ConnMax       int64 `stm:"conn_max"`

	// Lock metrics
	LockWaits         int64 `stm:"lock_waits"`
	LockTimeouts      int64 `stm:"lock_timeouts"`
	Deadlocks         int64 `stm:"deadlocks"`
	LockEscalations   int64 `stm:"lock_escalations"`
	LockActive        int64 `stm:"lock_active"`
	LockWaitTime      int64 `stm:"lock_wait_time"`
	LockWaitingAgents int64 `stm:"lock_waiting_agents"`
	LockMemoryPages   int64 `stm:"lock_memory_pages"`

	// Sort metrics
	TotalSorts    int64 `stm:"total_sorts"`
	SortOverflows int64 `stm:"sort_overflows"`

	// Row metrics
	RowsRead     int64 `stm:"rows_read"`
	RowsModified int64 `stm:"rows_modified"`
	RowsReturned int64 `stm:"rows_returned"`

	// Buffer pool hit/miss metrics for percentage-of-incremental-row
	BufferpoolHits          int64 `stm:"bufferpool_hits"`
	BufferpoolMisses        int64 `stm:"bufferpool_misses"`
	BufferpoolDataHits      int64 `stm:"bufferpool_data_hits"`
	BufferpoolDataMisses    int64 `stm:"bufferpool_data_misses"`
	BufferpoolIndexHits     int64 `stm:"bufferpool_index_hits"`
	BufferpoolIndexMisses   int64 `stm:"bufferpool_index_misses"`
	BufferpoolXDAHits       int64 `stm:"bufferpool_xda_hits"`
	BufferpoolXDAMisses     int64 `stm:"bufferpool_xda_misses"`
	BufferpoolColumnHits    int64 `stm:"bufferpool_column_hits"`
	BufferpoolColumnMisses  int64 `stm:"bufferpool_column_misses"`
	BufferpoolLogicalReads  int64 `stm:"bufferpool_logical_reads"`
	BufferpoolPhysicalReads int64 `stm:"bufferpool_physical_reads"`
	BufferpoolTotalReads    int64 // No stm tag - calculated field

	// Detailed buffer pool read metrics
	BufferpoolDataLogicalReads    int64 `stm:"bufferpool_data_logical_reads"`
	BufferpoolDataPhysicalReads   int64 `stm:"bufferpool_data_physical_reads"`
	BufferpoolDataTotalReads      int64 // No stm tag - calculated field
	BufferpoolIndexLogicalReads   int64 `stm:"bufferpool_index_logical_reads"`
	BufferpoolIndexPhysicalReads  int64 `stm:"bufferpool_index_physical_reads"`
	BufferpoolIndexTotalReads     int64 // No stm tag - calculated field
	BufferpoolXDALogicalReads     int64 `stm:"bufferpool_xda_logical_reads"`
	BufferpoolXDAPhysicalReads    int64 `stm:"bufferpool_xda_physical_reads"`
	BufferpoolXDATotalReads       int64 // No stm tag - calculated field
	BufferpoolColumnLogicalReads  int64 `stm:"bufferpool_column_logical_reads"`
	BufferpoolColumnPhysicalReads int64 `stm:"bufferpool_column_physical_reads"`
	BufferpoolColumnTotalReads    int64 // No stm tag - calculated field

	// Write metrics
	BufferpoolWrites int64 `stm:"bufferpool_writes"`

	// Log metrics
	LogUsedSpace      int64 `stm:"log_used_space"`
	LogAvailableSpace int64 `stm:"log_available_space"`
	LogUtilization    int64 `stm:"log_utilization"`
	LogIOReads        int64 `stm:"log_io_reads"`
	LogIOWrites       int64 `stm:"log_io_writes"`
	LogOpReads        int64 `stm:"log_op_reads"`
	LogOpWrites       int64 `stm:"log_op_writes"`

	// Query metrics
	LongRunningQueries         int64 `stm:"long_running_queries"`
	LongRunningQueriesWarning  int64 `stm:"long_running_queries_warning"`
	LongRunningQueriesCritical int64 `stm:"long_running_queries_critical"`

	// Backup metrics
	LastBackupStatus         int64 `stm:"last_backup_status"`
	LastFullBackupAge        int64 `stm:"last_full_backup_age"`
	LastIncrementalBackupAge int64 `stm:"last_incremental_backup_age"`

	// Service health checks
	CanConnect     int64 `stm:"can_connect"`
	DatabaseStatus int64 `stm:"database_status"`

	databases   map[string]databaseInstanceMetrics
	bufferpools map[string]bufferpoolInstanceMetrics
	tablespaces map[string]tablespaceInstanceMetrics
	connections map[string]connectionInstanceMetrics
	tables      map[string]tableInstanceMetrics
	indexes     map[string]indexInstanceMetrics
	memoryPools map[string]memoryPoolInstanceMetrics
	tableIOs    map[string]tableIOInstanceMetrics
	memorySets  map[string]memorySetInstanceMetrics
	prefetchers map[string]prefetcherInstanceMetrics
}

type databaseMetrics struct {
	name         string
	status       string
	applications int64
}

type bufferpoolMetrics struct {
	name     string
	pageSize int64
}

type tablespaceMetrics struct {
	name        string
	tbspType    string
	contentType string
	state       string
}

type connectionMetrics struct {
	applicationID   string
	applicationName string
	clientHostname  string
	clientIP        string
	clientUser      string
	connectionState string
}

type tableMetrics struct {
	name      string
	ioUpdated bool
}

type indexMetrics struct {
	name string
}

type memoryPoolMetrics struct {
	poolType string
	updated  bool
}

func (c *Collector) getTableMetrics(name string) *tableMetrics {
	if _, ok := c.tables[name]; !ok {
		c.tables[name] = &tableMetrics{name: name}
	}
	return c.tables[name]
}

func (c *Collector) getIndexMetrics(name string) *indexMetrics {
	if _, ok := c.indexes[name]; !ok {
		c.indexes[name] = &indexMetrics{name: name}
	}
	return c.indexes[name]
}

type databaseInstanceMetrics struct {
	Status       int64 `stm:"status"`
	Applications int64 `stm:"applications"`
}

type bufferpoolInstanceMetrics struct {
	// Basic metrics
	PageSize   int64 // No stm tag - used for labels only
	TotalPages int64 `stm:"total_pages"`
	UsedPages  int64 `stm:"used_pages"`

	// Hit/miss metrics for percentage-of-incremental-row
	Hits         int64 `stm:"hits"`
	Misses       int64 `stm:"misses"`
	DataHits     int64 `stm:"data_hits"`
	DataMisses   int64 `stm:"data_misses"`
	IndexHits    int64 `stm:"index_hits"`
	IndexMisses  int64 `stm:"index_misses"`
	XDAHits      int64 `stm:"xda_hits"`
	XDAMisses    int64 `stm:"xda_misses"`
	ColumnHits   int64 `stm:"column_hits"`
	ColumnMisses int64 `stm:"column_misses"`

	// Read metrics
	LogicalReads        int64 `stm:"logical_reads"`
	PhysicalReads       int64 `stm:"physical_reads"`
	TotalReads          int64 // No stm tag - calculated field
	DataLogicalReads    int64 `stm:"data_logical_reads"`
	DataPhysicalReads   int64 `stm:"data_physical_reads"`
	IndexLogicalReads   int64 `stm:"index_logical_reads"`
	IndexPhysicalReads  int64 `stm:"index_physical_reads"`
	XDALogicalReads     int64 `stm:"xda_logical_reads"`
	XDAPhysicalReads    int64 `stm:"xda_physical_reads"`
	ColumnLogicalReads  int64 `stm:"column_logical_reads"`
	ColumnPhysicalReads int64 `stm:"column_physical_reads"`

	// Write metrics
	Writes int64 `stm:"writes"`
}

type tablespaceInstanceMetrics struct {
	State       int64 `stm:"state"`
	TotalSize   int64 `stm:"total_size"`
	UsedSize    int64 `stm:"used_size"`
	FreeSize    int64 `stm:"free_size"`
	UsableSize  int64 `stm:"usable_size"`
	UsedPercent int64 `stm:"used_percent"`
	PageSize    int64 // No stm tag - used for labels only
}

type connectionInstanceMetrics struct {
	State            int64 `stm:"state"`
	ExecutingQueries int64 `stm:"executing_queries"`
	RowsRead         int64 `stm:"rows_read"`
	RowsWritten      int64 `stm:"rows_written"`
	TotalCPUTime     int64 `stm:"total_cpu_time"`

	// Wait time metrics (DB2 9.7+ LUW with MON_GET_CONNECTION)
	// TotalWaitTime is not charted but collected for reference
	TotalWaitTime     int64 // No stm tag - not sent to netdata
	LockWaitTime      int64 `stm:"lock_wait_time"`
	LogDiskWaitTime   int64 `stm:"log_disk_wait_time"`
	LogBufferWaitTime int64 `stm:"log_buffer_wait_time"`
	PoolReadTime      int64 `stm:"pool_read_time"`
	PoolWriteTime     int64 `stm:"pool_write_time"`
	DirectReadTime    int64 `stm:"direct_read_time"`
	DirectWriteTime   int64 `stm:"direct_write_time"`
	FCMRecvWaitTime   int64 `stm:"fcm_recv_wait_time"`
	FCMSendWaitTime   int64 `stm:"fcm_send_wait_time"`
	TotalRoutineTime  int64 `stm:"total_routine_time"`
	TotalCompileTime  int64 `stm:"total_compile_time"`
	TotalSectionTime  int64 `stm:"total_section_time"`
	TotalCommitTime   int64 `stm:"total_commit_time"`
	TotalRollbackTime int64 `stm:"total_rollback_time"`
}

type tableInstanceMetrics struct {
	DataSize    int64 `stm:"data_size"`
	IndexSize   int64 `stm:"index_size"`
	LongObjSize int64 `stm:"long_obj_size"`
	RowsRead    int64 `stm:"rows_read"`
	RowsWritten int64 `stm:"rows_written"`
}

type indexInstanceMetrics struct {
	LeafNodes  int64 `stm:"leaf_nodes"`
	IndexScans int64 `stm:"index_scans"`
	FullScans  int64 `stm:"full_scans"`
}

type memoryPoolInstanceMetrics struct {
	PoolUsed    int64 `stm:"used"`
	PoolUsedHWM int64 `stm:"hwm"`
}

type tableIOInstanceMetrics struct {
	TableScans       int64 `stm:"scans"`
	RowsRead         int64 `stm:"read"`
	RowsInserted     int64 `stm:"inserts"`
	RowsUpdated      int64 `stm:"updates"`
	RowsDeleted      int64 `stm:"deletes"`
	OverflowAccesses int64 `stm:"overflow_accesses"`
}

type memorySetInstanceMetrics struct {
	// Screen 26: Instance Memory Sets metrics
	Used                int64 `stm:"used"`                 // MEMORY_SET_USED
	Committed           int64 `stm:"committed"`            // MEMORY_SET_COMMITTED
	HighWaterMark       int64 `stm:"high_water_mark"`      // MEMORY_SET_USED_HWM
	AdditionalCommitted int64 `stm:"additional_committed"` // MEMORY_SET_COMMITTED - MEMORY_SET_USED
	PercentUsedHWM      int64 `stm:"percent_used_hwm"`     // (MEMORY_SET_USED * 100) / MEMORY_SET_USED_HWM

	// Metadata
	hostName string
	dbName   string
	setType  string
	member   int64
}

type prefetcherInstanceMetrics struct {
	PrefetchRatio int64 `stm:"prefetch_ratio"`
	CleanerRatio  int64 `stm:"cleaner_ratio"`
	PhysicalReads int64 `stm:"physical_reads"`
	AsyncReads    int64 `stm:"async_reads"`
	UnreadPages   int64 `stm:"unread_pages"`
	AvgWaitTime   int64 `stm:"avg_wait_time"`
}
