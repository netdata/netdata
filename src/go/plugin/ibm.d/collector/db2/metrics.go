// SPDX-License-Identifier: GPL-3.0-or-later

package db2

type metricsData struct {
	// Global metrics
	ConnTotal     int64 `stm:"conn_total"`
	ConnActive    int64 `stm:"conn_active"`
	ConnExecuting int64 `stm:"conn_executing"`
	ConnIdle      int64 `stm:"conn_idle"`

	LockWaits       int64 `stm:"lock_waits"`
	LockTimeouts    int64 `stm:"lock_timeouts"`
	Deadlocks       int64 `stm:"deadlocks"`
	LockEscalations int64 `stm:"lock_escalations"`

	TotalSorts    int64 `stm:"total_sorts"`
	SortOverflows int64 `stm:"sort_overflows"`

	RowsRead     int64 `stm:"rows_read"`
	RowsModified int64 `stm:"rows_modified"`

	BufferpoolHitRatio int64 `stm:"bufferpool_hit_ratio"`

	LogUsedSpace      int64 `stm:"log_used_space"`
	LogAvailableSpace int64 `stm:"log_available_space"`

	// Long-running queries
	LongRunningQueries        int64 `stm:"long_running_queries"`
	LongRunningQueriesWarning int64 `stm:"long_running_queries_warning"`
	LongRunningQueriesCritical int64 `stm:"long_running_queries_critical"`

	// Backup status
	LastFullBackupAge      int64 `stm:"last_full_backup_age"`      // hours
	LastIncrementalBackupAge int64 `stm:"last_incremental_backup_age"` // hours
	LastBackupStatus       int64 `stm:"last_backup_status"`       // 0=success, 1=failed

	// Per-instance metrics
	databases   map[string]databaseInstanceMetrics
	bufferpools map[string]bufferpoolInstanceMetrics
	tablespaces map[string]tablespaceInstanceMetrics
	connections map[string]connectionInstanceMetrics
}

type databaseMetrics struct {
	name         string
	status       string
	applications int64
}

type databaseInstanceMetrics struct {
	Status       int64 `stm:"status"`
	Applications int64 `stm:"applications"`
}

type bufferpoolMetrics struct {
	name     string
	pageSize int64
}

type bufferpoolInstanceMetrics struct {
	HitRatio   int64 `stm:"hit_ratio"`
	Reads      int64 `stm:"reads"`
	Writes     int64 `stm:"writes"`
	PageSize   int64 `stm:"page_size"`
	TotalPages int64 `stm:"total_pages"`
	UsedPages  int64 `stm:"used_pages"`
}

type tablespaceMetrics struct {
	name        string
	tbspType    string
	contentType string
	state       string
}

type tablespaceInstanceMetrics struct {
	State       int64 `stm:"state"`
	TotalSizeKB int64 `stm:"total_size_kb"`
	UsedSizeKB  int64 `stm:"used_size_kb"`
	FreeSizeKB  int64 `stm:"free_size_kb"`
	UsedPercent int64 `stm:"used_percent"`
	PageSize    int64 `stm:"page_size"`
}

type connectionMetrics struct {
	applicationID   string
	applicationName string
	clientHostname  string
	clientUser      string
	connectionState string
}

type connectionInstanceMetrics struct {
	State            int64 `stm:"state"`
	ExecutingQueries int64 `stm:"executing_queries"`
	RowsRead         int64 `stm:"rows_read"`
	RowsWritten      int64 `stm:"rows_written"`
	TotalCPUTime     int64 `stm:"total_cpu_time"`
}
