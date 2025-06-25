// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

const precision = 1000 // Precision multiplier for floating-point values

func (d *DB2) collect(ctx context.Context) (map[string]int64, error) {
	if d.db == nil {
		db, err := d.initDatabase(ctx)
		if err != nil {
			return nil, err
		}
		d.db = db

		// Detect DB2 version on first connection
		if err := d.detectVersion(ctx); err != nil {
			d.Warningf("failed to detect DB2 version: %v", err)
		} else {
			d.Infof("detected DB2 version: %s edition: %s", d.version, d.edition)
		}
	}

	// Reset metrics
	d.mx = &metricsData{
		databases:   make(map[string]databaseInstanceMetrics),
		bufferpools: make(map[string]bufferpoolInstanceMetrics),
		tablespaces: make(map[string]tablespaceInstanceMetrics),
		connections: make(map[string]connectionInstanceMetrics),
	}

	// Test connection with a ping before proceeding
	if err := d.ping(ctx); err != nil {
		// Try to reconnect once
		d.Cleanup(ctx)
		db, err := d.initDatabase(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to reconnect to database: %v", err)
		}
		d.db = db

		// Retry ping
		if err := d.ping(ctx); err != nil {
			return nil, fmt.Errorf("database connection failed after reconnect: %v", err)
		}

		// Re-detect version after reconnect
		if err := d.detectVersion(ctx); err != nil {
			d.Warningf("failed to detect DB2 version after reconnect: %v", err)
		}
	}

	// Collect global metrics
	if err := d.collectGlobalMetrics(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect global metrics: %v", err)
	}

	// Collect per-instance metrics if enabled
	if d.CollectDatabaseMetrics {
		if err := d.collectDatabaseInstances(ctx); err != nil {
			d.Errorf("failed to collect database instances: %v", err)
		}
	}

	if d.CollectBufferpoolMetrics {
		if err := d.collectBufferpoolInstances(ctx); err != nil {
			d.Errorf("failed to collect bufferpool instances: %v", err)
		}
	}

	if d.CollectTablespaceMetrics {
		if err := d.collectTablespaceInstances(ctx); err != nil {
			d.Errorf("failed to collect tablespace instances: %v", err)
		}
	}

	if d.CollectConnectionMetrics {
		if err := d.collectConnectionInstances(ctx); err != nil {
			d.Errorf("failed to collect connection instances: %v", err)
		}
	}

	// Cleanup stale instances
	d.cleanupStaleInstances()

	// Build final metrics map
	mx := stm.ToMap(d.mx)

	// Add per-instance metrics
	for name, metrics := range d.mx.databases {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("database_%s_%s", cleanName, k)] = v
		}
	}

	for name, metrics := range d.mx.bufferpools {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("bufferpool_%s_%s", cleanName, k)] = v
		}
	}

	for name, metrics := range d.mx.tablespaces {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("tablespace_%s_%s", cleanName, k)] = v
		}
	}

	for id, metrics := range d.mx.connections {
		cleanID := cleanName(id)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("connection_%s_%s", cleanID, k)] = v
		}
	}

	return mx, nil
}

func (d *DB2) detectVersion(ctx context.Context) error {
	// Try SYSIBMADM.ENV_INST_INFO (works on LUW)
	query := `SELECT SERVICE_LEVEL, HOST_NAME, INST_NAME FROM SYSIBMADM.ENV_INST_INFO`

	var serviceLevel, hostName, instName sql.NullString
	err := d.db.QueryRow(query).Scan(&serviceLevel, &hostName, &instName)
	if err == nil {
		d.serverInfo.version = serviceLevel.String
		d.serverInfo.hostName = hostName.String
		d.serverInfo.instanceName = instName.String

		// Parse version to determine edition
		if strings.Contains(serviceLevel.String, "DB2") {
			if strings.Contains(serviceLevel.String, "LUW") || strings.Contains(serviceLevel.String, "Linux") || strings.Contains(serviceLevel.String, "Windows") {
				d.edition = "LUW"
			} else if strings.Contains(serviceLevel.String, "z/OS") {
				d.edition = "z/OS"
			} else {
				d.edition = "LUW" // Default to LUW
			}
		}
		d.version = serviceLevel.String
		return nil
	}

	// If that fails, might be AS/400 (DB2 for i)
	query = `SELECT 'DB2 for i' FROM SYSIBM.SYSDUMMY1`
	var dummy string
	err = d.db.QueryRow(query).Scan(&dummy)
	if err == nil {
		d.edition = "i"
		d.version = "DB2 for i"
		return nil
	}

	return fmt.Errorf("unable to detect DB2 version")
}

func (d *DB2) collectGlobalMetrics(ctx context.Context) error {
	// Connection counts - works across all DB2 versions
	query := `
		SELECT 
			COUNT(*) as TOTAL_CONNS,
			SUM(CASE WHEN APPL_STATUS = 'CONNECTED' THEN 1 ELSE 0 END) as ACTIVE_CONNS,
			SUM(CASE WHEN APPL_STATUS = 'UOWEXEC' THEN 1 ELSE 0 END) as EXECUTING_CONNS
		FROM SYSIBMADM.APPLICATIONS
	`

	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnTotal = v
			}
		case "ACTIVE_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnActive = v
			}
		case "EXECUTING_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnExecuting = v
			}
		}
	})

	// Calculate idle connections
	d.mx.ConnIdle = d.mx.ConnActive - d.mx.ConnExecuting

	if err != nil {
		// Try simpler query for older versions
		simpleQuery := `SELECT COUNT(*) FROM SYSIBMADM.APPLICATIONS`
		if err2 := d.doQuerySingleValue(ctx, simpleQuery, &d.mx.ConnTotal); err2 != nil {
			return fmt.Errorf("failed to get connection count: %v", err)
		}
	}

	// Lock metrics
	if err := d.collectLockMetrics(ctx); err != nil {
		d.Warningf("failed to collect lock metrics: %v", err)
	}

	// Buffer pool aggregate hit ratio
	if err := d.collectBufferpoolAggregateMetrics(ctx); err != nil {
		d.Warningf("failed to collect bufferpool metrics: %v", err)
	}

	// Log space metrics
	if err := d.collectLogSpaceMetrics(ctx); err != nil {
		d.Warningf("failed to collect log space metrics: %v", err)
	}

	// Long-running queries
	if err := d.collectLongRunningQueries(ctx); err != nil {
		d.Warningf("failed to collect long-running queries: %v", err)
	}

	// Backup status
	if err := d.collectBackupStatus(ctx); err != nil {
		d.Warningf("failed to collect backup status: %v", err)
	}

	return nil
}

func (d *DB2) collectLockMetrics(ctx context.Context) error {
	query := `
		SELECT 
			LOCK_WAITS,
			LOCK_TIMEOUTS,
			DEADLOCKS,
			LOCK_ESCALS,
			TOTAL_SORTS,
			SORT_OVERFLOWS,
			ROWS_READ,
			ROWS_MODIFIED
		FROM SYSIBMADM.SNAPDB
	`

	return d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "LOCK_WAITS":
			d.mx.LockWaits = v
		case "LOCK_TIMEOUTS":
			d.mx.LockTimeouts = v
		case "DEADLOCKS":
			d.mx.Deadlocks = v
		case "LOCK_ESCALS":
			d.mx.LockEscalations = v
		case "TOTAL_SORTS":
			d.mx.TotalSorts = v
		case "SORT_OVERFLOWS":
			d.mx.SortOverflows = v
		case "ROWS_READ":
			d.mx.RowsRead = v
		case "ROWS_MODIFIED":
			d.mx.RowsModified = v
		}
	})
}

func (d *DB2) collectBufferpoolAggregateMetrics(ctx context.Context) error {
	query := `
		SELECT 
			CASE 
				WHEN (POOL_DATA_L_READS + POOL_INDEX_L_READS) > 0 
				THEN ((POOL_DATA_LBP_PAGES_FOUND + POOL_INDEX_LBP_PAGES_FOUND) * 100) / 
				     (POOL_DATA_L_READS + POOL_INDEX_L_READS)
				ELSE 100 
			END as HIT_RATIO
		FROM SYSIBMADM.SNAPBP
	`

	return d.doQuerySingleFloatValue(ctx, query, &d.mx.BufferpoolHitRatio)
}

func (d *DB2) collectLogSpaceMetrics(ctx context.Context) error {
	query := `
		SELECT 
			TOTAL_LOG_USED,
			TOTAL_LOG_AVAILABLE
		FROM SYSIBMADM.LOG_UTILIZATION
	`

	return d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "TOTAL_LOG_USED":
			d.mx.LogUsedSpace = v
		case "TOTAL_LOG_AVAILABLE":
			d.mx.LogAvailableSpace = v
		}
	})
}

func (d *DB2) doQuery(ctx context.Context, query string, assign func(column, value string, lineEnd bool)) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	rows, err := d.db.QueryContext(queryCtx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	return d.readRows(rows, assign)
}

func (d *DB2) doQuerySingleValue(ctx context.Context, query string, target *int64) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	var value sql.NullInt64
	err := d.db.QueryRowContext(queryCtx, query).Scan(&value)
	if err != nil {
		return err
	}
	if value.Valid {
		*target = value.Int64
	}
	return nil
}

func (d *DB2) doQuerySingleFloatValue(ctx context.Context, query string, target *int64) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	var value sql.NullFloat64
	err := d.db.QueryRowContext(queryCtx, query).Scan(&value)
	if err != nil {
		return err
	}
	if value.Valid {
		*target = int64(value.Float64 * precision)
	}
	return nil
}

func (d *DB2) readRows(rows *sql.Rows, assign func(column, value string, lineEnd bool)) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := make([]sql.NullString, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}

		for i, column := range columns {
			if values[i].Valid {
				assign(column, values[i].String, i == len(columns)-1)
			} else {
				assign(column, "", i == len(columns)-1)
			}
		}
	}

	return rows.Err()
}

func (d *DB2) collectLongRunningQueries(ctx context.Context) error {
	// Query to find long-running queries from SYSIBMADM.LONG_RUNNING_SQL
	// Warning threshold: 5 minutes, Critical threshold: 15 minutes
	query := `
		SELECT 
			COUNT(*) as TOTAL_COUNT,
			SUM(CASE WHEN ELAPSED_TIME_MIN >= 5 AND ELAPSED_TIME_MIN < 15 THEN 1 ELSE 0 END) as WARNING_COUNT,
			SUM(CASE WHEN ELAPSED_TIME_MIN >= 15 THEN 1 ELSE 0 END) as CRITICAL_COUNT
		FROM SYSIBMADM.LONG_RUNNING_SQL
		WHERE ELAPSED_TIME_MIN > 0
	`

	return d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "TOTAL_COUNT":
			d.mx.LongRunningQueries = v
		case "WARNING_COUNT":
			d.mx.LongRunningQueriesWarning = v
		case "CRITICAL_COUNT":
			d.mx.LongRunningQueriesCritical = v
		}
	})
}

func (d *DB2) collectBackupStatus(ctx context.Context) error {
	// Query backup history from SYSIBMADM.DB_HISTORY
	// Note: This table structure varies between DB2 versions
	query := `
		SELECT 
			OPERATION,
			START_TIME,
			OPERATIONTYPE,
			SQLCODE
		FROM SYSIBMADM.DB_HISTORY
		WHERE OPERATION = 'B'
		ORDER BY START_TIME DESC
		FETCH FIRST 10 ROWS ONLY
	`

	var (
		lastFullBackupTime        time.Time
		lastIncrementalBackupTime time.Time
		lastBackupFailed          bool
		currentStartTime          time.Time
		currentOperationType      string
	)

	now := time.Now()

	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "START_TIME":
			// Parse backup time
			if t, err := time.Parse("2006-01-02-15.04.05", value); err == nil {
				currentStartTime = t
			}
		case "OPERATIONTYPE":
			currentOperationType = value
		case "SQLCODE":
			// SQLCODE 0 = success, anything else = failure
			if value == "0" && !currentStartTime.IsZero() {
				// Successful backup
				if currentOperationType == "F" && lastFullBackupTime.IsZero() {
					lastFullBackupTime = currentStartTime
				} else if (currentOperationType == "I" || currentOperationType == "O" || currentOperationType == "D") && lastIncrementalBackupTime.IsZero() {
					lastIncrementalBackupTime = currentStartTime
				}
			} else if value != "0" && value != "" {
				lastBackupFailed = true
			}
		}

		if lineEnd {
			// Reset for next row
			currentStartTime = time.Time{}
			currentOperationType = ""
		}
	})

	if err != nil {
		// Try simpler approach - just get the latest backup timestamp
		simpleQuery := `
			SELECT 
				MAX(START_TIME) as LAST_BACKUP
			FROM SYSIBMADM.DB_HISTORY
			WHERE OPERATION = 'B' AND SQLCODE = 0
		`
		var lastBackup sql.NullString
		queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
		defer cancel()

		if err := d.db.QueryRowContext(queryCtx, simpleQuery).Scan(&lastBackup); err == nil && lastBackup.Valid {
			if t, err := time.Parse("2006-01-02-15.04.05", lastBackup.String); err == nil {
				lastFullBackupTime = t
			}
		}
	}

	// Calculate age in hours
	if !lastFullBackupTime.IsZero() {
		d.mx.LastFullBackupAge = int64(now.Sub(lastFullBackupTime).Hours())
	} else {
		d.mx.LastFullBackupAge = 999999 // Very old/never
	}

	if !lastIncrementalBackupTime.IsZero() {
		d.mx.LastIncrementalBackupAge = int64(now.Sub(lastIncrementalBackupTime).Hours())
	} else {
		d.mx.LastIncrementalBackupAge = 999999 // Very old/never
	}

	if lastBackupFailed {
		d.mx.LastBackupStatus = 1
	} else {
		d.mx.LastBackupStatus = 0
	}

	return nil
}
