// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

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

const Precision = 1000 // Precision multiplier for floating-point values

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}
	c.db = c.client.DB()

	// Reset metrics
	c.mx = &metricsData{
		databases:   make(map[string]databaseInstanceMetrics),
		bufferpools: make(map[string]bufferpoolInstanceMetrics),
		tablespaces: make(map[string]tablespaceInstanceMetrics),
		connections: make(map[string]connectionInstanceMetrics),
		tables:      make(map[string]tableInstanceMetrics),
		indexes:     make(map[string]indexInstanceMetrics),
		memoryPools: make(map[string]memoryPoolInstanceMetrics),
		memorySets:  make(map[string]memorySetInstanceMetrics),
		tableIOs:    make(map[string]tableIOInstanceMetrics),
	}

	// Collect global metrics
	if err := c.collectGlobalMetrics(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect global metrics: %v", err)
	}

	// Collect per-instance metrics if enabled and supported
	if c.CollectDatabaseMetrics.IsEnabled() {
		c.Debugf("collecting database instance metrics (limit: %d)", c.MaxDatabases)
		if err := c.collectDatabaseInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("database_instances_unavailable", "Database instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect database instances: %v", err)
			}
		}
	}

	if c.CollectBufferpoolMetrics.IsEnabled() {
		c.Debugf("collecting bufferpool instance metrics (limit: %d)", c.MaxBufferpools)
		if err := c.collectBufferpoolInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("bufferpool_instances_unavailable", "Bufferpool instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect bufferpool instances: %v", err)
			}
		}
	}

	if c.CollectTablespaceMetrics.IsEnabled() {
		c.Debugf("collecting tablespace instance metrics (limit: %d)", c.MaxTablespaces)
		if err := c.collectTablespaceInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("tablespace_instances_unavailable", "Tablespace instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect tablespace instances: %v", err)
			}
		}
	}

	if c.CollectConnectionMetrics.IsEnabled() {
		c.Debugf("collecting connection instance metrics (limit: %d)", c.MaxConnections)
		if err := c.collectConnectionInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("connection_instances_unavailable", "Connection instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect connection instances: %v", err)
			}
		}
	}

	if c.CollectTableMetrics.IsEnabled() {
		c.Debugf("collecting table instance metrics (limit: %d)", c.MaxTables)
		if err := c.collectTableInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("table_instances_unavailable", "Table instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect table instances: %v", err)
			}
		}
	}

	if c.CollectIndexMetrics.IsEnabled() {
		c.Debugf("collecting index instance metrics (limit: %d)", c.MaxIndexes)
		if err := c.collectIndexInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("index_instances_unavailable", "Index instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect index instances: %v", err)
			}
		}
	}

	// Collect new performance metrics

	if c.CollectMemoryMetrics {
		c.Debugf("collecting memory pool metrics")
		if err := c.collectMemoryPoolInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("memory_pool_unavailable", "Memory pool collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect memory pools: %v", err)
			}
		}

		// Screen 26: Collect Instance Memory Sets
		c.Debugf("collecting memory set instances (Screen 26)")
		if err := c.collectMemorySetInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("memory_set_unavailable", "Memory set collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect memory sets: %v", err)
			}
		}

		// Screen 15: Collect Prefetchers
		c.Debugf("collecting prefetcher instances (Screen 15)")
		if err := c.collectPrefetcherInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("prefetcher_unavailable", "Prefetcher collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect prefetchers: %v", err)
			}
		}
	}

	if c.CollectWaitMetrics && c.CollectConnectionMetrics.IsEnabled() {
		c.Debugf("collecting enhanced wait metrics")
		if err := c.collectConnectionWaits(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("wait_metrics_unavailable", "Wait metrics collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect wait metrics: %v", err)
			}
		}
	}

	if c.CollectTableIOMetrics {
		c.Debugf("collecting table I/O metrics")
		if err := c.collectTableIOInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				c.logOnce("table_io_unavailable", "Table I/O collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				c.Errorf("failed to collect table I/O: %v", err)
			}
		}
	}

	// Build final metrics map
	mx := stm.ToMap(c.mx)

	// Add per-instance metrics
	for name, metrics := range c.mx.databases {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("database_%s_%s", cleanName, k)] = v
		}
	}

	// Debug bufferpool count
	if len(c.mx.bufferpools) > 0 {
		c.Debugf("Processing %d bufferpools in mx", len(c.mx.bufferpools))
	}

	for name, metrics := range c.mx.bufferpools {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("bufferpool_%s_%s", cleanName, k)] = v
		}

		// Calculate hit ratios for instance charts
		// Overall hit ratio
		totalReads := metrics.Hits + metrics.Misses
		if totalReads > 0 {
			hitRatio := float64(metrics.Hits) * 100.0 / float64(totalReads)
			mx[fmt.Sprintf("bufferpool_%s_hit_ratio", cleanName)] = int64(hitRatio * Precision)
		} else {
			// No reads means 100% hit ratio (no misses)
			mx[fmt.Sprintf("bufferpool_%s_hit_ratio", cleanName)] = 100 * Precision
		}

		// Data hit ratio
		dataReads := metrics.DataHits + metrics.DataMisses
		if dataReads > 0 {
			dataHitRatio := float64(metrics.DataHits) * 100.0 / float64(dataReads)
			mx[fmt.Sprintf("bufferpool_%s_data_hit_ratio", cleanName)] = int64(dataHitRatio * Precision)
		} else {
			mx[fmt.Sprintf("bufferpool_%s_data_hit_ratio", cleanName)] = 100 * Precision
		}

		// Index hit ratio
		indexReads := metrics.IndexHits + metrics.IndexMisses
		if indexReads > 0 {
			indexHitRatio := float64(metrics.IndexHits) * 100.0 / float64(indexReads)
			mx[fmt.Sprintf("bufferpool_%s_index_hit_ratio", cleanName)] = int64(indexHitRatio * Precision)
		} else {
			mx[fmt.Sprintf("bufferpool_%s_index_hit_ratio", cleanName)] = 100 * Precision
		}

		// XDA hit ratio
		xdaReads := metrics.XDAHits + metrics.XDAMisses
		if xdaReads > 0 {
			xdaHitRatio := float64(metrics.XDAHits) * 100.0 / float64(xdaReads)
			mx[fmt.Sprintf("bufferpool_%s_xda_hit_ratio", cleanName)] = int64(xdaHitRatio * Precision)
		} else {
			mx[fmt.Sprintf("bufferpool_%s_xda_hit_ratio", cleanName)] = 100 * Precision
		}

		// Column hit ratio
		columnReads := metrics.ColumnHits + metrics.ColumnMisses
		if columnReads > 0 {
			columnHitRatio := float64(metrics.ColumnHits) * 100.0 / float64(columnReads)
			mx[fmt.Sprintf("bufferpool_%s_column_hit_ratio", cleanName)] = int64(columnHitRatio * Precision)
		} else {
			mx[fmt.Sprintf("bufferpool_%s_column_hit_ratio", cleanName)] = 100 * Precision
		}

		// Debug
		c.Debugf("Bufferpool %s: hits=%d, misses=%d, dataHits=%d, dataMisses=%d",
			name, metrics.Hits, metrics.Misses, metrics.DataHits, metrics.DataMisses)
	}

	for name, metrics := range c.mx.tablespaces {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("tablespace_%s_%s", cleanName, k)] = v
		}
	}

	for id, metrics := range c.mx.connections {
		cleanID := cleanName(id)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("connection_%s_%s", cleanID, k)] = v
		}
	}

	for name, metrics := range c.mx.tables {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("table_%s_%s", cleanName, k)] = v
		}
	}

	for name, metrics := range c.mx.indexes {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("index_%s_%s", cleanName, k)] = v
		}
	}

	// Add new metric types

	for poolType, metrics := range c.mx.memoryPools {
		cleanType := cleanName(poolType)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("memory_pool_%s_%s", cleanType, k)] = v
		}
	}

	for tableName, metrics := range c.mx.tableIOs {
		cleanName := cleanName(tableName)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("table_io_%s_%s", cleanName, k)] = v
		}
	}

	// Add memory set metrics
	for setKey, metrics := range c.memorySets {
		// Split the key to apply cleanName to each component
		parts := strings.Split(setKey, ".")
		if len(parts) >= 3 {
			// Apply cleanName to match chart dimension IDs
			setIdentifier := fmt.Sprintf("%s_%s_%s",
				cleanName(parts[0]), // host name
				cleanName(parts[1]), // db name
				cleanName(parts[2])) // set type
			// Add member if present
			if len(parts) >= 4 {
				setIdentifier += "_" + parts[3] // member number
			}
			for k, v := range stm.ToMap(metrics) {
				mx[fmt.Sprintf("memory_set_%s_%s", setIdentifier, k)] = v
			}
		}
	}

	// Add prefetcher metrics
	for bufferPoolName, metrics := range c.prefetchers {
		cleanName := cleanName(bufferPoolName)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("prefetcher_%s_%s", cleanName, k)] = v
		}
	}

	return mx, nil
}

func (c *Collector) collectServiceHealth(ctx context.Context) {
	// Connection check
	c.mx.CanConnect = 0
	if err := c.doQuerySingleValue(ctx, queryCanConnect, &c.mx.CanConnect); err != nil {
		c.mx.CanConnect = 0
	}

	// Database status check
	// 0 = OK (active), 1 = WARNING (quiesce-pending, rollforward), 2 = CRITICAL (quiesced), 3 = UNKNOWN
	c.mx.DatabaseStatus = 3
	if err := c.doQuerySingleValue(ctx, queryDatabaseStatus, &c.mx.DatabaseStatus); err != nil {
		c.mx.DatabaseStatus = 3
	}
}

func (c *Collector) detectVersion(ctx context.Context) error {
	// Try SYSIBMADM.ENV_INST_INFO (works on LUW)
	query := queryDetectVersionLUW

	var serviceLevel, hostName, instName sql.NullString
	err := c.db.QueryRow(query).Scan(&serviceLevel, &hostName, &instName)
	if err == nil {
		c.serverInfo.version = serviceLevel.String
		c.serverInfo.hostName = hostName.String
		c.serverInfo.instanceName = instName.String

		// Parse version to determine edition
		if strings.Contains(serviceLevel.String, "DB2") {
			if strings.Contains(serviceLevel.String, "LUW") || strings.Contains(serviceLevel.String, "Linux") || strings.Contains(serviceLevel.String, "Windows") {
				c.edition = "LUW"
			} else if strings.Contains(serviceLevel.String, "z/OS") {
				c.edition = "z/OS"
			} else {
				c.edition = "LUW" // Default to LUW
			}
		}
		c.version = serviceLevel.String
		return nil
	}

	// If that fails, might be AS/400 (DB2 for i)
	query = queryDetectVersionI
	var dummy sql.NullString
	err = c.db.QueryRow(query).Scan(&dummy)
	if err == nil {
		c.edition = "i"
		c.version = "DB2 for i"
		return nil
	}

	return fmt.Errorf("unable to detect DB2 version")
}

func (c *Collector) collectGlobalMetrics(ctx context.Context) error {
	c.Debugf("starting global metrics collection")

	// Service health checks - core functionality
	if err := c.collectServiceHealthResilience(ctx); err != nil {
		return err // Service health is critical
	}

	// Connection metrics - core functionality that should always work
	if err := c.collectConnectionMetricsResilience(ctx); err != nil {
		return err // Connection metrics are critical
	}

	// Database Overview metrics (Screen 01) - always collect if possible
	if err := c.collectDatabaseOverview(ctx); err != nil {
		c.Warningf("failed to collect database overview metrics: %v", err)
	}

	// Enhanced Logging Performance metrics (Screen 18)
	if err := c.collectLoggingPerformance(ctx); err != nil {
		c.Warningf("failed to collect enhanced logging performance metrics: %v", err)
	}

	// Federation metrics (Screen 32) - only if supported
	if err := c.collectFederationMetrics(ctx); err != nil {
		// Not logging as warning since federation might not be configured
		c.Debugf("federation metrics collection skipped: %v", err)
	}

	// Always use modern MON_GET_* functions
	// Collect all database-level metrics in one efficient call
	if !c.isDisabled("advanced_monitoring") {
		if err := c.collectMonGetDatabase(ctx); err != nil {
			c.Warningf("failed to collect database metrics using MON_GET_DATABASE: %v", err)
			// Try individual metric collection as fallback
			c.collectLockMetricsResilience(ctx)
			c.collectSortingMetricsResilience(ctx)
			c.collectRowActivityMetricsResilience(ctx)
		}
	} else {
		c.logOnce("advanced_monitoring_skipped", "Advanced monitoring metrics collection skipped - Not available on this DB2 edition/version")
	}

	// Buffer pool metrics using MON_GET_BUFFERPOOL
	if !c.isDisabled("bufferpool_detailed_metrics") {
		if err := c.collectMonGetBufferpoolAggregate(ctx); err != nil {
			c.Warningf("failed to collect bufferpool metrics using MON_GET_BUFFERPOOL: %v", err)
			c.collectBufferpoolMetricsResilience(ctx)
		}
	} else {
		c.logOnce("bufferpool_detailed_skipped", "Detailed buffer pool metrics collection skipped - Limited on this DB2 edition")
	}

	// Log space metrics using MON_GET_TRANSACTION_LOG
	if !c.isDisabled("system_level_metrics") {
		if err := c.collectMonGetTransactionLog(ctx); err != nil {
			c.Warningf("failed to collect log metrics using MON_GET_TRANSACTION_LOG: %v", err)
			c.collectLogSpaceMetricsResilience(ctx)
		}
	} else {
		c.logOnce("system_level_skipped", "System-level metrics collection skipped - Restricted on this DB2 edition")
	}

	// Long-running queries and backup status - collected separately as they don't have MON_GET equivalents
	if !c.isDisabled("advanced_monitoring") {
		// Long-running queries - graceful degradation
		c.collectLongRunningQueriesResilience(ctx)

		// Backup status - graceful degradation
		c.collectBackupStatusResilience(ctx)
	}

	c.Debugf("completed global metrics collection")
	return nil
}

func (c *Collector) collectLockMetrics(ctx context.Context) error {
	// Choose query based on monitoring approach
	// Always use modern MON_GET_DATABASE for lock metrics
	query := queryMonGetDatabase
	c.Debugf("using MON_GET_DATABASE for lock metrics")

	return c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "LOCK_WAITS":
			c.mx.LockWaits = v
		case "LOCK_TIMEOUTS":
			c.mx.LockTimeouts = v
		case "DEADLOCKS":
			c.mx.Deadlocks = v
		case "LOCK_ESCALS":
			c.mx.LockEscalations = v
		case "LOCK_ACTIVE":
			c.mx.LockActive = v
		case "LOCK_WAIT_TIME":
			c.mx.LockWaitTime = v * Precision // Convert to milliseconds with Precision
		case "LOCK_WAITING_AGENTS":
			c.mx.LockWaitingAgents = v
		case "LOCK_MEMORY_PAGES":
			c.mx.LockMemoryPages = v
		case "TOTAL_SORTS":
			c.mx.TotalSorts = v
		case "SORT_OVERFLOWS":
			c.mx.SortOverflows = v
		case "ROWS_READ":
			c.mx.RowsRead = v
		case "ROWS_MODIFIED":
			c.mx.RowsModified = v
		case "ROWS_RETURNED":
			c.mx.RowsReturned = v
		}
	})
}

func (c *Collector) collectBufferpoolAggregateMetrics(ctx context.Context) error {
	// Always use modern MON_GET_BUFFERPOOL for aggregate metrics
	c.Debugf("using MON_GET_BUFFERPOOL for aggregate metrics")
	return c.collectMonGetBufferpoolAggregate(ctx)
}

func (c *Collector) collectLogSpaceMetrics(ctx context.Context) error {
	// Always use MON_GET_TRANSACTION_LOG for log metrics
	query := queryMonGetTransactionLog
	c.Debugf("using MON_GET_TRANSACTION_LOG for log metrics")

	return c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_LOG_USED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogUsedSpace = v
			}
		case "TOTAL_LOG_AVAILABLE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogAvailableSpace = v
			}
		case "LOG_UTILIZATION":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				c.mx.LogUtilization = int64(v * Precision)
			}
		case "LOG_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogIOReads = v
			}
		case "LOG_WRITES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogIOWrites = v
			}
		}
	})
}

func (c *Collector) doQuery(ctx context.Context, query string, assign func(column, value string, lineEnd bool)) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout))
	defer cancel()

	rows, err := c.db.QueryContext(queryCtx, query)
	if err != nil {
		if isSQLFeatureError(err) {
			c.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		} else {
			c.Errorf("failed to execute query: %s, error: %v", query, err)
		}
		return err
	}
	defer rows.Close()

	return c.readRows(rows, assign)
}

func (c *Collector) doQuerySingleValue(ctx context.Context, query string, target *int64) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout))
	defer cancel()

	var value sql.NullInt64
	err := c.db.QueryRowContext(queryCtx, query).Scan(&value)
	if err != nil {
		if isSQLFeatureError(err) {
			c.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		}
		return err
	}
	if value.Valid {
		*target = value.Int64
	}
	return nil
}

func (c *Collector) doQuerySingleFloatValue(ctx context.Context, query string, target *int64) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout))
	defer cancel()

	var value sql.NullFloat64
	err := c.db.QueryRowContext(queryCtx, query).Scan(&value)
	if err != nil {
		if isSQLFeatureError(err) {
			c.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		}
		return err
	}
	if value.Valid {
		*target = int64(value.Float64 * Precision)
	}
	return nil
}

func (c *Collector) readRows(rows *sql.Rows, assign func(column, value string, lineEnd bool)) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := make([]sql.NullString, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Track which query is being processed for better error reporting
	var currentQuery string
	if len(columns) > 0 {
		// Try to identify query by column pattern
		switch {
		case contains(columns, "MEMORY_SET_TYPE", "MEMORY_SET_USED"):
			currentQuery = "MON_GET_MEMORY_SET"
		case contains(columns, "MEMORY_POOL_TYPE", "MEMORY_POOL_USED"):
			currentQuery = "MON_GET_MEMORY_POOL"
		default:
			currentQuery = "unknown"
		}
	}

	// Universal fix for ODBC driver issue with DB2/AS400 negative values
	defer func() {
		if r := recover(); r != nil {
			// This is a known ODBC driver bug where it incorrectly handles certain DB2 data types
			// The driver attempts to use negative values as slice indices, causing panics
			c.Debugf("ODBC driver panic in %s query (columns: %v): %v", currentQuery, columns, r)
			c.Debugf("This is a known ODBC driver limitation with certain DB2 data types")
		}
	}()

	for rows.Next() {
		// Wrap Scan in panic recovery as well since it can also trigger ODBC issues
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.Debugf("ODBC scan panic recovered: %v", r)
				}
			}()

			if err := rows.Scan(valuePtrs...); err != nil {
				c.Debugf("Row scan error: %v", err)
				return
			}

			for i, column := range columns {
				if values[i].Valid {
					assign(column, values[i].String, i == len(columns)-1)
				} else {
					assign(column, "", i == len(columns)-1)
				}
			}
		}()
	}

	return rows.Err()
}

func (c *Collector) collectLongRunningQueries(ctx context.Context) error {
	// Query to find long-running queries from SYSIBMADM.LONG_RUNNING_SQL
	// Warning threshold: 5 minutes, Critical threshold: 15 minutes
	return c.doQuery(ctx, queryLongRunningQueries, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "TOTAL_COUNT":
			c.mx.LongRunningQueries = v
		case "WARNING_COUNT":
			c.mx.LongRunningQueriesWarning = v
		case "CRITICAL_COUNT":
			c.mx.LongRunningQueriesCritical = v
		}
	})
}

func (c *Collector) collectBackupStatus(ctx context.Context) error {
	now := time.Now()

	// First check if we have ANY backup (successful or failed) in the last 7 days
	var lastBackupSQLCode sql.NullInt64
	var lastBackupTime sql.NullString
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Timeout))
	defer cancel()

	// Get the most recent backup attempt with ODBC-safe error handling
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.Debugf("backup history query failed due to ODBC driver issue: %v", r)
				err = fmt.Errorf("odbc driver error: %v", r)
			}
		}()
		err = c.db.QueryRowContext(queryCtx, `
			SELECT SQLCODE, START_TIME
			FROM SYSIBMADM.DB_HISTORY 
			WHERE OPERATION = 'B' 
			  AND OPERATIONTYPE = 'F'
			ORDER BY START_TIME DESC
			FETCH FIRST 1 ROW ONLY
		`).Scan(&lastBackupSQLCode, &lastBackupTime)
	}()

	if err == nil && lastBackupSQLCode.Valid {
		// Check if the last backup was successful (SQLCODE = 0)
		if lastBackupSQLCode.Int64 == 0 {
			c.mx.LastBackupStatus = 0 // Success
		} else {
			c.mx.LastBackupStatus = 1 // Failed (non-zero SQLCODE)
		}
	} else {
		// No backup history found
		c.mx.LastBackupStatus = 0 // Don't raise alert if no backup history
	}

	// Get the last successful full backup with ODBC-safe error handling
	var lastFullBackup sql.NullString
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.Debugf("full backup query failed due to ODBC driver issue: %v", r)
				err = fmt.Errorf("odbc driver error: %v", r)
			}
		}()
		err = c.db.QueryRowContext(queryCtx, `
			SELECT MAX(START_TIME) 
			FROM SYSIBMADM.DB_HISTORY 
			WHERE OPERATION = 'B' 
			  AND OPERATIONTYPE = 'F' 
			  AND SQLCODE = 0
		`).Scan(&lastFullBackup)
	}()

	if err == nil && lastFullBackup.Valid {
		if t, err := time.Parse("2006-01-02-15.04.05", lastFullBackup.String); err == nil {
			c.mx.LastFullBackupAge = int64(now.Sub(t).Hours())
		} else {
			c.Warningf("failed to parse last full backup time '%s': %v (expected format: YYYY-MM-DD-HH.MM.SS)", lastFullBackup.String, err)
			c.mx.LastFullBackupAge = 0 // Parse error - report 0 hours (recent)
		}
	} else {
		// No successful backup found
		c.mx.LastFullBackupAge = 720 // 30 days in hours - old but reasonable
	}

	// Get the last successful incremental backup with ODBC-safe error handling
	var lastIncrementalBackup sql.NullString
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.Debugf("incremental backup query failed due to ODBC driver issue: %v", r)
				err = fmt.Errorf("odbc driver error: %v", r)
			}
		}()
		err = c.db.QueryRowContext(queryCtx, `
			SELECT MAX(START_TIME) 
			FROM SYSIBMADM.DB_HISTORY 
			WHERE OPERATION = 'B' 
			  AND OPERATIONTYPE IN ('I', 'O', 'D') 
			  AND SQLCODE = 0
		`).Scan(&lastIncrementalBackup)
	}()

	if err == nil && lastIncrementalBackup.Valid {
		if t, err := time.Parse("2006-01-02-15.04.05", lastIncrementalBackup.String); err == nil {
			c.mx.LastIncrementalBackupAge = int64(now.Sub(t).Hours())
		} else {
			c.Warningf("failed to parse last incremental backup time '%s': %v (expected format: YYYY-MM-DD-HH.MM.SS)", lastIncrementalBackup.String, err)
			c.mx.LastIncrementalBackupAge = 0 // Parse error - report 0 hours (recent)
		}
	} else {
		// No incremental backup found - this is normal for many setups
		c.mx.LastIncrementalBackupAge = 0 // Report 0 to avoid false alerts
	}

	return nil
}

// Resilient collection functions following AS/400 pattern

func (c *Collector) collectServiceHealthResilience(ctx context.Context) error {
	// Service health is critical - these must work
	if err := c.collectSingleMetric(ctx, "can_connect", queryCanConnect, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.CanConnect = v
		}
	}); err != nil {
		return err // Fatal - basic connectivity must work
	}

	// Database status - optional on some editions
	_ = c.collectSingleMetric(ctx, "database_status", queryDatabaseStatus, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.DatabaseStatus = v
		}
	})

	return nil
}

func (c *Collector) collectConnectionMetricsResilience(ctx context.Context) error {
	// Always try MON_GET first for better performance
	if err := c.collectMonGetConnections(ctx); err == nil {
		return nil
	}

	// Fall back to individual queries if MON_GET fails
	// Core connection metrics - must work on all DB2 editions
	if err := c.collectSingleMetric(ctx, "total_connections", queryTotalConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.ConnTotal = v
		}
	}); err != nil {
		return err // Fatal - basic connection count must work
	}

	// Optional connection breakdowns - graceful degradation
	_ = c.collectSingleMetric(ctx, "active_connections", queryActiveConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.ConnActive = v
		}
	})

	_ = c.collectSingleMetric(ctx, "executing_connections", queryExecutingConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.ConnExecuting = v
		}
	})

	_ = c.collectSingleMetric(ctx, "idle_connections", queryIdleConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.ConnIdle = v
		}
	})

	_ = c.collectSingleMetric(ctx, "max_connections", queryMaxConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.ConnMax = v
		}
	})

	// Calculate idle if not available directly
	if c.mx.ConnIdle == 0 && c.mx.ConnActive > 0 {
		c.mx.ConnIdle = c.mx.ConnActive - c.mx.ConnExecuting
	}

	return nil
}

func (c *Collector) collectLockMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for lock metrics
	_ = c.collectSingleMetric(ctx, "lock_waits", queryLockWaits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockWaits = v
		}
	})

	_ = c.collectSingleMetric(ctx, "lock_timeouts", queryLockTimeouts, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockTimeouts = v
		}
	})

	_ = c.collectSingleMetric(ctx, "deadlocks", queryDeadlocks, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.Deadlocks = v
		}
	})

	_ = c.collectSingleMetric(ctx, "lock_escalations", queryLockEscalations, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockEscalations = v
		}
	})

	_ = c.collectSingleMetric(ctx, "active_locks", queryActiveLocks, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockActive = v
		}
	})

	_ = c.collectSingleMetric(ctx, "lock_wait_time", queryLockWaitTime, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockWaitTime = v * Precision // Convert to milliseconds with Precision
		}
	})

	_ = c.collectSingleMetric(ctx, "lock_waiting_agents", queryLockWaitingAgents, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockWaitingAgents = v
		}
	})

	_ = c.collectSingleMetric(ctx, "lock_memory_pages", queryLockMemoryPages, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LockMemoryPages = v
		}
	})
}

func (c *Collector) collectSortingMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for sorting metrics
	_ = c.collectSingleMetric(ctx, "total_sorts", queryTotalSorts, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.TotalSorts = v
		}
	})

	_ = c.collectSingleMetric(ctx, "sort_overflows", querySortOverflows, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.SortOverflows = v
		}
	})
}

func (c *Collector) collectRowActivityMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for row activity metrics
	_ = c.collectSingleMetric(ctx, "rows_read", queryRowsRead, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.RowsRead = v
		}
	})

	_ = c.collectSingleMetric(ctx, "rows_modified", queryRowsModified, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.RowsModified = v
		}
	})

	_ = c.collectSingleMetric(ctx, "rows_returned", queryRowsReturned, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.RowsReturned = v
		}
	})
}

func (c *Collector) collectBufferpoolMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for bufferpool metrics
	// Collect individual components first
	var dataLogical, dataHits int64
	var indexLogical, indexHits int64
	var xdaLogical, xdaHits int64
	var colLogical, colHits int64

	// Get reads
	_ = c.collectSingleMetric(ctx, "bufferpool_logical_reads", queryBufferpoolLogicalReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.BufferpoolLogicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_physical_reads", queryBufferpoolPhysicalReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.BufferpoolPhysicalReads = v
		}
	})

	// Data reads
	_ = c.collectSingleMetric(ctx, "bufferpool_data_logical", queryBufferpoolDataLogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			dataLogical = v
			c.mx.BufferpoolDataLogicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_data_physical", queryBufferpoolDataPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.BufferpoolDataPhysicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_data_hits", queryBufferpoolDataHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			dataHits = v
		}
	})

	// Index reads
	_ = c.collectSingleMetric(ctx, "bufferpool_index_logical", queryBufferpoolIndexLogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			indexLogical = v
			c.mx.BufferpoolIndexLogicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_index_physical", queryBufferpoolIndexPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.BufferpoolIndexPhysicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_index_hits", queryBufferpoolIndexHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			indexHits = v
		}
	})

	// XDA reads
	_ = c.collectSingleMetric(ctx, "bufferpool_xda_logical", queryBufferpoolXDALogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			xdaLogical = v
			c.mx.BufferpoolXDALogicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_xda_physical", queryBufferpoolXDAPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.BufferpoolXDAPhysicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_xda_hits", queryBufferpoolXDAHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			xdaHits = v
		}
	})

	// Column reads
	_ = c.collectSingleMetric(ctx, "bufferpool_column_logical", queryBufferpoolColumnLogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			colLogical = v
			c.mx.BufferpoolColumnLogicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_column_physical", queryBufferpoolColumnPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.BufferpoolColumnPhysicalReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "bufferpool_column_hits", queryBufferpoolColumnHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			colHits = v
		}
	})

	// Calculate hit ratios from components
	// Calculate hits and misses for each type
	c.mx.BufferpoolDataHits = dataHits
	c.mx.BufferpoolDataMisses = dataLogical - dataHits

	c.mx.BufferpoolIndexHits = indexHits
	c.mx.BufferpoolIndexMisses = indexLogical - indexHits

	c.mx.BufferpoolXDAHits = xdaHits
	c.mx.BufferpoolXDAMisses = xdaLogical - xdaHits

	c.mx.BufferpoolColumnHits = colHits
	c.mx.BufferpoolColumnMisses = colLogical - colHits

	// If misses are negative, it means prefetch brought more pages than were requested
	// In this case, set misses to 0 and reduce hits accordingly
	if c.mx.BufferpoolDataMisses < 0 {
		c.mx.BufferpoolDataHits = dataLogical
		c.mx.BufferpoolDataMisses = 0
	}
	if c.mx.BufferpoolIndexMisses < 0 {
		c.mx.BufferpoolIndexHits = indexLogical
		c.mx.BufferpoolIndexMisses = 0
	}
	if c.mx.BufferpoolXDAMisses < 0 {
		c.mx.BufferpoolXDAHits = xdaLogical
		c.mx.BufferpoolXDAMisses = 0
	}
	if c.mx.BufferpoolColumnMisses < 0 {
		c.mx.BufferpoolColumnHits = colLogical
		c.mx.BufferpoolColumnMisses = 0
	}

	// Calculate overall hits and misses
	c.mx.BufferpoolHits = c.mx.BufferpoolDataHits + c.mx.BufferpoolIndexHits +
		c.mx.BufferpoolXDAHits + c.mx.BufferpoolColumnHits
	c.mx.BufferpoolMisses = c.mx.BufferpoolDataMisses + c.mx.BufferpoolIndexMisses +
		c.mx.BufferpoolXDAMisses + c.mx.BufferpoolColumnMisses
}

func (c *Collector) collectLogSpaceMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for log space metrics
	var logUsed, logAvailable int64

	_ = c.collectSingleMetric(ctx, "log_used_space", queryLogUsedSpace, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			logUsed = v
			c.mx.LogUsedSpace = v
		}
	})

	_ = c.collectSingleMetric(ctx, "log_available_space", queryLogAvailableSpace, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			logAvailable = v
			c.mx.LogAvailableSpace = v
		}
	})

	_ = c.collectSingleMetric(ctx, "log_reads", queryLogReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LogIOReads = v
		}
	})

	_ = c.collectSingleMetric(ctx, "log_writes", queryLogWrites, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LogIOWrites = v
		}
	})

	// Calculate log utilization
	if logUsed > 0 && logAvailable > 0 {
		total := logUsed + logAvailable
		c.mx.LogUtilization = int64((float64(logUsed) * 100.0 * float64(Precision)) / float64(total))
	}
}

func (c *Collector) collectLongRunningQueriesResilience(ctx context.Context) {
	_ = c.collectSingleMetric(ctx, "long_running_total", queryLongRunningTotal, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LongRunningQueries = v
		}
	})

	_ = c.collectSingleMetric(ctx, "long_running_warning", queryLongRunningWarning, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LongRunningQueriesWarning = v
		}
	})

	_ = c.collectSingleMetric(ctx, "long_running_critical", queryLongRunningCritical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LongRunningQueriesCritical = v
		}
	})
}

func (c *Collector) collectBackupStatusResilience(ctx context.Context) {
	// This will use the existing collectBackupStatus function as it's already resilient
	_ = c.collectBackupStatus(ctx)
}

// MON_GET collection functions for modern monitoring approach

func (c *Collector) collectMonGetConnections(ctx context.Context) error {
	// First get connection counts
	err := c.doQuery(ctx, queryMonGetConnections, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.ConnTotal = v
			}
		case "ACTIVE_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.ConnActive = v
			}
		case "IDLE_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.ConnIdle = v
			}
		case "EXECUTING_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.ConnExecuting = v
			}
		}
	})

	if err != nil {
		return err
	}

	// Then get max connections from configuration
	return c.collectSingleMetric(ctx, "max_connections", queryMaxConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.ConnMax = v
		}
	})
}

func (c *Collector) collectMonGetDatabase(ctx context.Context) error {
	return c.doQuery(ctx, queryMonGetDatabase, func(column, value string, lineEnd bool) {
		switch column {
		case "LOCK_WAITS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockWaits = v
			}
		case "LOCK_TIMEOUTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockTimeouts = v
			}
		case "DEADLOCKS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.Deadlocks = v
			}
		case "LOCK_ESCALS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockEscalations = v
			}
		case "LOCK_ACTIVE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockActive = v
			}
		case "LOCK_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockWaitTime = v
			}
		case "LOCK_WAITING_AGENTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockWaitingAgents = v
			}
		case "LOCK_MEMORY_PAGES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockMemoryPages = v
			}
		case "TOTAL_SORTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.TotalSorts = v
			}
		case "SORT_OVERFLOWS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.SortOverflows = v
			}
		case "ROWS_READ":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.RowsRead = v
			}
		case "ROWS_MODIFIED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.RowsModified = v
			}
		case "ROWS_RETURNED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.RowsReturned = v
			}
		}
	})
}

func (c *Collector) collectMonGetBufferpoolAggregate(ctx context.Context) error {
	var dataLogical, dataPhysical, dataHits int64
	var indexLogical, indexPhysical, indexHits int64
	var xdaLogical, xdaPhysical, xdaHits int64
	var colLogical, colPhysical, colHits int64

	err := c.doQuery(ctx, queryMonGetBufferpoolAggregate, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "DATA_LOGICAL_READS":
			dataLogical = v
			c.mx.BufferpoolDataLogicalReads = v
		case "DATA_PHYSICAL_READS":
			dataPhysical = v
			c.mx.BufferpoolDataPhysicalReads = v
		case "DATA_HITS":
			dataHits = v
		case "INDEX_LOGICAL_READS":
			indexLogical = v
			c.mx.BufferpoolIndexLogicalReads = v
		case "INDEX_PHYSICAL_READS":
			indexPhysical = v
			c.mx.BufferpoolIndexPhysicalReads = v
		case "INDEX_HITS":
			indexHits = v
		case "XDA_LOGICAL_READS":
			xdaLogical = v
			c.mx.BufferpoolXDALogicalReads = v
		case "XDA_PHYSICAL_READS":
			xdaPhysical = v
			c.mx.BufferpoolXDAPhysicalReads = v
		case "XDA_HITS":
			xdaHits = v
		case "COLUMN_LOGICAL_READS":
			colLogical = v
			c.mx.BufferpoolColumnLogicalReads = v
		case "COLUMN_PHYSICAL_READS":
			colPhysical = v
			c.mx.BufferpoolColumnPhysicalReads = v
		case "COLUMN_HITS":
			colHits = v
		case "TOTAL_WRITES":
			c.mx.BufferpoolWrites = v
		}
	})

	if err != nil {
		return err
	}

	// Calculate totals
	c.mx.BufferpoolLogicalReads = dataLogical + indexLogical + xdaLogical + colLogical
	c.mx.BufferpoolPhysicalReads = dataPhysical + indexPhysical + xdaPhysical + colPhysical
	c.mx.BufferpoolTotalReads = c.mx.BufferpoolLogicalReads + c.mx.BufferpoolPhysicalReads

	// Calculate data totals
	c.mx.BufferpoolDataTotalReads = dataLogical + dataPhysical
	c.mx.BufferpoolIndexTotalReads = indexLogical + indexPhysical
	c.mx.BufferpoolXDATotalReads = xdaLogical + xdaPhysical
	c.mx.BufferpoolColumnTotalReads = colLogical + colPhysical

	// Calculate hits and misses for each type
	c.mx.BufferpoolDataHits = dataHits
	c.mx.BufferpoolDataMisses = dataLogical - dataHits

	c.mx.BufferpoolIndexHits = indexHits
	c.mx.BufferpoolIndexMisses = indexLogical - indexHits

	c.mx.BufferpoolXDAHits = xdaHits
	c.mx.BufferpoolXDAMisses = xdaLogical - xdaHits

	c.mx.BufferpoolColumnHits = colHits
	c.mx.BufferpoolColumnMisses = colLogical - colHits

	// If misses are negative, it means prefetch brought more pages than were requested
	// In this case, set misses to 0 and reduce hits accordingly
	if c.mx.BufferpoolDataMisses < 0 {
		c.mx.BufferpoolDataHits = dataLogical
		c.mx.BufferpoolDataMisses = 0
	}
	if c.mx.BufferpoolIndexMisses < 0 {
		c.mx.BufferpoolIndexHits = indexLogical
		c.mx.BufferpoolIndexMisses = 0
	}
	if c.mx.BufferpoolXDAMisses < 0 {
		c.mx.BufferpoolXDAHits = xdaLogical
		c.mx.BufferpoolXDAMisses = 0
	}
	if c.mx.BufferpoolColumnMisses < 0 {
		c.mx.BufferpoolColumnHits = colLogical
		c.mx.BufferpoolColumnMisses = 0
	}

	// Calculate overall hits and misses
	c.mx.BufferpoolHits = c.mx.BufferpoolDataHits + c.mx.BufferpoolIndexHits +
		c.mx.BufferpoolXDAHits + c.mx.BufferpoolColumnHits
	c.mx.BufferpoolMisses = c.mx.BufferpoolDataMisses + c.mx.BufferpoolIndexMisses +
		c.mx.BufferpoolXDAMisses + c.mx.BufferpoolColumnMisses

	return nil
}

func (c *Collector) collectMonGetTransactionLog(ctx context.Context) error {
	return c.doQuery(ctx, queryMonGetTransactionLog, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_LOG_USED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogUsedSpace = v
			}
		case "TOTAL_LOG_AVAILABLE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogAvailableSpace = v
			}
		case "LOG_UTILIZATION":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				c.mx.LogUtilization = int64(v * Precision)
			}
		case "LOG_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogIOReads = v
			}
		case "LOG_WRITES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogIOWrites = v
			}
		}
	})
}

// Database Overview collection (Screen 01)
func (c *Collector) collectDatabaseOverview(ctx context.Context) error {
	// Use simple queries approach (based on dcmtop) for more resilience
	// Run multiple simple queries instead of one complex query

	// Helper function to run a simple query and handle errors gracefully
	// It tries the MON_GET query first, then falls back to SNAP query if that fails
	runSimpleQuery := func(queryMonGet, querySnap string, handler func(column, value string)) {
		var err error
		if queryMonGet != "" {
			err = c.doQuery(ctx, queryMonGet, func(column, value string, lineEnd bool) {
				handler(column, value)
			})
			if err != nil && isSQLFeatureError(err) {
				c.Debugf("MON_GET query not supported, trying SNAP query: %v", err)
				if querySnap != "" {
					err = c.doQuery(ctx, querySnap, func(column, value string, lineEnd bool) {
						handler(column, value)
					})
				}
			}
		} else if querySnap != "" {
			err = c.doQuery(ctx, querySnap, func(column, value string, lineEnd bool) {
				handler(column, value)
			})
		}
		if err != nil {
			c.Debugf("query failed (will continue): %v", err)
		}
	}

	// Database status (current connected database)
	runSimpleQuery(querySimpleDatabaseStatus, querySnapDatabaseStatus, func(column, value string) {
		if column == "DATABASE_STATUS" && value == "ACTIVE" {
			c.mx.DatabaseStatusActive = 1
			c.mx.DatabaseStatusInactive = 0
		} else {
			c.mx.DatabaseStatusActive = 0
			c.mx.DatabaseStatusInactive = 1
		}
	})

	// Database count (all databases in the instance)
	// This will be updated by collectDatabaseInstances()
	// Initialize to zero here
	c.mx.DatabaseCountActive = 0
	c.mx.DatabaseCountInactive = 0

	// CPU metrics
	// Temporary storage for raw nanosecond values
	var cpuUserNs, cpuSystemNs, cpuIdleNs, cpuIowaitNs float64

	runSimpleQuery(querySimpleCPUSystem, "", func(column, value string) {
		switch column {
		case "CPU_USER_TOTAL":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				cpuUserNs = v // Store raw nanoseconds
			}
		case "CPU_SYSTEM_TOTAL":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				cpuSystemNs = v
			}
		case "CPU_IDLE_TOTAL":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				cpuIdleNs = v
			}
		case "CPU_IOWAIT_TOTAL":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				cpuIowaitNs = v
			}
		}
	})

	// Convert nanoseconds to percentages
	totalNs := cpuUserNs + cpuSystemNs + cpuIdleNs + cpuIowaitNs
	if totalNs > 0 {
		// Calculate percentages and apply Precision for storage
		c.mx.CPUUser = int64((cpuUserNs / totalNs) * 100 * Precision)
		c.mx.CPUSystem = int64((cpuSystemNs / totalNs) * 100 * Precision)
		c.mx.CPUIdle = int64((cpuIdleNs / totalNs) * 100 * Precision)
		c.mx.CPUIowait = int64((cpuIowaitNs / totalNs) * 100 * Precision)
	}

	// Connection metrics
	runSimpleQuery(querySimpleConnectionsActive, querySnapConnectionsActive, func(column, value string) {
		if column == "ACTIVE_CONNECTIONS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.ConnectionsActive = v
			}
		}
	})

	runSimpleQuery(querySimpleConnectionsTotal, querySnapConnectionsTotal, func(column, value string) {
		if column == "TOTAL_CONNECTIONS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.ConnectionsTotal = v
			}
		}
	})

	// Memory metrics
	runSimpleQuery(querySimpleMemoryInstance, "", func(column, value string) {
		if column == "INSTANCE_MEM_COMMITTED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.MemoryInstanceCommitted = v
			}
		}
	})

	runSimpleQuery(querySimpleMemoryDatabase, "", func(column, value string) {
		if column == "DATABASE_MEM_COMMITTED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.MemoryDatabaseCommitted = v
			}
		}
	})

	runSimpleQuery(querySimpleMemoryBufferpool, "", func(column, value string) {
		if column == "BUFFERPOOL_MEM_USED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.MemoryBufferpoolUsed = v
			}
		}
	})

	runSimpleQuery(querySimpleMemorySharedSort, "", func(column, value string) {
		if column == "SHARED_SORT_MEM_USED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.MemorySharedSortUsed = v
			}
		}
	})

	// Throughput metrics
	runSimpleQuery(querySimpleTransactions, querySnapTransactions, func(column, value string) {
		if column == "TRANSACTIONS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.OpsTransactions = v
			}
		}
	})

	runSimpleQuery(querySimpleSelectStmts, querySnapSelectStmts, func(column, value string) {
		if column == "SELECT_STMTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.OpsSelectStmts = v
			}
		}
	})

	runSimpleQuery(querySimpleUIDStmts, querySnapUIDStmts, func(column, value string) {
		if column == "UID_STMTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.OpsUIDStmts = v
			}
		}
	})

	runSimpleQuery(querySimpleActivitiesAborted, querySnapActivitiesAborted, func(column, value string) {
		if column == "ACTIVITIES_ABORTED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.OpsActivitiesAborted = v
			}
		}
	})

	// Time spent metrics
	runSimpleQuery(querySimpleAvgDirectReadTime, querySnapAvgDirectReadTime, func(column, value string) {
		if column == "AVG_DIRECT_READ_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				c.mx.TimeAvgDirectRead = int64(v * Precision * 1000) // Convert to microseconds
			}
		}
	})

	runSimpleQuery(querySimpleAvgDirectWriteTime, querySnapAvgDirectWriteTime, func(column, value string) {
		if column == "AVG_DIRECT_WRITE_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				c.mx.TimeAvgDirectWrite = int64(v * Precision * 1000)
			}
		}
	})

	runSimpleQuery(querySimpleAvgPoolReadTime, querySnapAvgPoolReadTime, func(column, value string) {
		if column == "AVG_POOL_READ_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				c.mx.TimeAvgPoolRead = int64(v * Precision * 1000)
			}
		}
	})

	runSimpleQuery(querySimpleAvgPoolWriteTime, querySnapAvgPoolWriteTime, func(column, value string) {
		if column == "AVG_POOL_WRITE_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				c.mx.TimeAvgPoolWrite = int64(v * Precision * 1000)
			}
		}
	})

	// Additional metrics that were not in the original complex query
	// but are collected by dcmtop

	// Lock metrics
	runSimpleQuery(querySimpleLockHeld, querySnapLockHeld, func(column, value string) {
		if column == "LOCKS_HELD" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockActive = v
			}
		}
	})

	runSimpleQuery(querySimpleLockWaits, querySnapLockWaits, func(column, value string) {
		if column == "LOCK_WAITS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockWaits = v
			}
		}
	})

	runSimpleQuery(querySimpleLockTimeouts, querySnapLockTimeouts, func(column, value string) {
		if column == "LOCK_TIMEOUTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LockTimeouts = v
			}
		}
	})

	runSimpleQuery(querySimpleDeadlocks, querySnapDeadlocks, func(column, value string) {
		if column == "DEADLOCKS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.Deadlocks = v
			}
		}
	})

	// Log operations
	runSimpleQuery(querySimpleLogReads, querySnapLogReads, func(column, value string) {
		if column == "LOG_READS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogOpReads = v
			}
		}
	})

	runSimpleQuery(querySimpleLogWrites, querySnapLogWrites, func(column, value string) {
		if column == "LOG_WRITES" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.LogOpWrites = v
			}
		}
	})

	// Sorts
	runSimpleQuery(querySimpleSorts, querySnapSorts, func(column, value string) {
		if column == "SORTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.TotalSorts = v
			}
		}
	})

	runSimpleQuery(querySimpleSortOverflows, querySnapSortOverflows, func(column, value string) {
		if column == "SORT_OVERFLOWS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.mx.SortOverflows = v
			}
		}
	})

	return nil // Always return nil since we run queries individually and handle errors gracefully
}

// Enhanced Logging Performance collection (Screen 18)
func (c *Collector) collectLoggingPerformance(ctx context.Context) error {
	// Use individual tested queries from dcmtop instead of complex combined queries

	// Collect log commits (from TOTAL_APP_COMMITS)
	_ = c.collectSingleMetric(ctx, "log_commits", queryLogCommits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LogCommits = v
		}
	})

	// Collect log rollbacks (from TOTAL_APP_ROLLBACKS)
	_ = c.collectSingleMetric(ctx, "log_rollbacks", queryLogRollbacks, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LogRollbacks = v
		}
	})

	// Collect log I/O reads (from NUM_LOG_READ_IO)
	_ = c.collectSingleMetric(ctx, "log_io_reads", queryLoggingReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LogIOReads = v
		}
	})

	// Collect log I/O writes (from NUM_LOG_WRITE_IO)
	_ = c.collectSingleMetric(ctx, "log_io_writes", queryLoggingWrites, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.mx.LogIOWrites = v
		}
	})

	return nil
}

// Federation metrics collection removed
// The federation queries contained static values and have been removed
func (c *Collector) collectFederationMetrics(ctx context.Context) error {
	// Federation support was removed along with Cloud-specific queries
	return nil
}

// contains is a helper function to check if all target strings are present in the slice
func contains(slice []string, targets ...string) bool {
	for _, target := range targets {
		found := false
		for _, s := range slice {
			if s == target {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
