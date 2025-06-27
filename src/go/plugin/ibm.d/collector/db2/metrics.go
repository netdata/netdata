// SPDX-License-Identifier: GPL-3.0-or-later

package db2

type metricsData struct {
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

	// Buffer pool aggregate metrics
	BufferpoolHitRatio       int64 `stm:"bufferpool_hit_ratio"`
	BufferpoolDataHitRatio   int64 `stm:"bufferpool_data_hit_ratio"`
	BufferpoolIndexHitRatio  int64 `stm:"bufferpool_index_hit_ratio"`
	BufferpoolXDAHitRatio    int64 `stm:"bufferpool_xda_hit_ratio"`
	BufferpoolColumnHitRatio int64 `stm:"bufferpool_column_hit_ratio"`
	BufferpoolLogicalReads   int64 `stm:"bufferpool_logical_reads"`
	BufferpoolPhysicalReads  int64 `stm:"bufferpool_physical_reads"`
	BufferpoolTotalReads     int64 `stm:"bufferpool_total_reads"`

	// Detailed buffer pool read metrics
	BufferpoolDataLogicalReads    int64 `stm:"bufferpool_data_logical_reads"`
	BufferpoolDataPhysicalReads   int64 `stm:"bufferpool_data_physical_reads"`
	BufferpoolDataTotalReads      int64 `stm:"bufferpool_data_total_reads"`
	BufferpoolIndexLogicalReads   int64 `stm:"bufferpool_index_logical_reads"`
	BufferpoolIndexPhysicalReads  int64 `stm:"bufferpool_index_physical_reads"`
	BufferpoolIndexTotalReads     int64 `stm:"bufferpool_index_total_reads"`
	BufferpoolXDALogicalReads     int64 `stm:"bufferpool_xda_logical_reads"`
	BufferpoolXDAPhysicalReads    int64 `stm:"bufferpool_xda_physical_reads"`
	BufferpoolXDATotalReads       int64 `stm:"bufferpool_xda_total_reads"`
	BufferpoolColumnLogicalReads  int64 `stm:"bufferpool_column_logical_reads"`
	BufferpoolColumnPhysicalReads int64 `stm:"bufferpool_column_physical_reads"`
	BufferpoolColumnTotalReads    int64 `stm:"bufferpool_column_total_reads"`

	// Log metrics
	LogUsedSpace      int64 `stm:"log_used_space"`
	LogAvailableSpace int64 `stm:"log_available_space"`
	LogUtilization    int64 `stm:"log_utilization"`
	LogReads          int64 `stm:"log_reads"`
	LogWrites         int64 `stm:"log_writes"`

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
	clientUser      string
	connectionState string
}

type tableMetrics struct {
	name string
}

type indexMetrics struct {
	name string
}

func (d *DB2) getTableMetrics(name string) *tableMetrics {
	if _, ok := d.tables[name]; !ok {
		d.tables[name] = &tableMetrics{name: name}
	}
	return d.tables[name]
}

func (d *DB2) getIndexMetrics(name string) *indexMetrics {
	if _, ok := d.indexes[name]; !ok {
		d.indexes[name] = &indexMetrics{name: name}
	}
	return d.indexes[name]
}

type databaseInstanceMetrics struct {
	Status       int64 `stm:"status"`
	Applications int64 `stm:"applications"`
}

type bufferpoolInstanceMetrics struct {
	// Basic metrics
	PageSize   int64 `stm:"page_size"`
	TotalPages int64 `stm:"total_pages"`
	UsedPages  int64 `stm:"used_pages"`

	// Hit ratios
	HitRatio       int64 `stm:"hit_ratio"`
	DataHitRatio   int64 `stm:"data_hit_ratio"`
	IndexHitRatio  int64 `stm:"index_hit_ratio"`
	XDAHitRatio    int64 `stm:"xda_hit_ratio"`
	ColumnHitRatio int64 `stm:"column_hit_ratio"`

	// Read metrics
	LogicalReads        int64 `stm:"logical_reads"`
	PhysicalReads       int64 `stm:"physical_reads"`
	TotalReads          int64 `stm:"total_reads"`
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
	PageSize    int64 `stm:"page_size"`
}

type connectionInstanceMetrics struct {
	State            int64 `stm:"state"`
	ExecutingQueries int64 `stm:"executing_queries"`
	RowsRead         int64 `stm:"rows_read"`
	RowsWritten      int64 `stm:"rows_written"`
	TotalCPUTime     int64 `stm:"total_cpu_time"`
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
