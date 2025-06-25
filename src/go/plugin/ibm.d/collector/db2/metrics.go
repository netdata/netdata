// SPDX-License-Identifier: GPL-3.0-or-later

package db2

type metricsData struct {
	ConnTotal      int64 `stm:"conn_total"`
	ConnActive     int64 `stm:"conn_active"`
	ConnExecuting  int64 `stm:"conn_executing"`
	ConnIdle       int64 `stm:"conn_idle"`
	LockWaits      int64 `stm:"lock_waits"`
	LockTimeouts   int64 `stm:"lock_timeouts"`
	Deadlocks      int64 `stm:"deadlocks"`
	LockEscalations int64 `stm:"lock_escalations"`
	TotalSorts     int64 `stm:"total_sorts"`
	SortOverflows  int64 `stm:"sort_overflows"`
	RowsRead       int64 `stm:"rows_read"`
	RowsModified   int64 `stm:"rows_modified"`
	BufferpoolHitRatio int64 `stm:"bufferpool_hit_ratio"`
	LogUsedSpace      int64 `stm:"log_used_space"`
	LogAvailableSpace int64 `stm:"log_available_space"`
	LongRunningQueries         int64 `stm:"long_running_queries"`
	LongRunningQueriesWarning  int64 `stm:"long_running_queries_warning"`
	LongRunningQueriesCritical int64 `stm:"long_running_queries_critical"`
	LastBackupStatus         int64 `stm:"last_backup_status"`
	LastFullBackupAge        int64 `stm:"last_full_backup_age"`
	LastIncrementalBackupAge int64 `stm:"last_incremental_backup_age"`

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
	Reads      int64 `stm:"reads"`
	Writes     int64 `stm:"writes"`
	HitRatio   int64 `stm:"hit_ratio"`
	PageSize   int64 `stm:"page_size"`
	TotalPages int64 `stm:"total_pages"`
	UsedPages  int64 `stm:"used_pages"`
}

type tablespaceInstanceMetrics struct {
	State       int64 `stm:"state"`
	TotalSizeKB int64 `stm:"total_size_kb"`
	UsedSizeKB  int64 `stm:"used_size_kb"`
	FreeSizeKB  int64 `stm:"free_size_kb"`
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
	LeafNodes   int64 `stm:"leaf_nodes"`
	IndexScans  int64 `stm:"index_scans"`
	FullScans   int64 `stm:"full_scans"`
}