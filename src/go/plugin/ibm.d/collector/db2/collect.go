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

const Precision = 1000 // Precision multiplier for floating-point values

func (d *DB2) collect(ctx context.Context) (map[string]int64, error) {
	if d.db == nil {
		db, err := d.initDatabase(ctx)
		if err != nil {
			return nil, err
		}
		d.db = db

		// Detect DB2 edition and version on first connection
		if err := d.detectDB2Edition(ctx); err != nil {
			d.Warningf("failed to detect DB2 edition: %v", err)
		} else {
			d.Infof("detected DB2 edition: %s version: %s", d.edition, d.version)
		}

		// Set configuration defaults based on detected version (only if admin hasn't configured)
		d.setConfigurationDefaults()

		// Check if we can use modern MON_GET_* functions
		d.detectMonGetSupport(ctx)

		// Check if column-organized table metrics are available
		d.detectColumnOrganizedSupport(ctx)
		
		// Check if extended metrics are available
		d.detectExtendedMetricsSupport(ctx)
		
		// Check if federation is configured
		d.detectFederationSupport(ctx)
	}

	// Reset metrics
	d.mx = &metricsData{
		databases:   make(map[string]databaseInstanceMetrics),
		bufferpools: make(map[string]bufferpoolInstanceMetrics),
		tablespaces: make(map[string]tablespaceInstanceMetrics),
		connections: make(map[string]connectionInstanceMetrics),
		tables:      make(map[string]tableInstanceMetrics),
		indexes:     make(map[string]indexInstanceMetrics),
		statements:  make(map[string]statementInstanceMetrics),
		memoryPools: make(map[string]memoryPoolInstanceMetrics),
		memorySets:  make(map[string]memorySetInstanceMetrics),
		tableIOs:    make(map[string]tableIOInstanceMetrics),
	}

	// Test connection with a ping before proceeding
	if err := d.ping(ctx); err != nil {
		d.Infof("connection test failed, attempting to reconnect: %v", err)
		// Try to reconnect once
		d.Cleanup(ctx)
		db, err := d.initDatabase(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to reconnect to database after connection test failure: %v", err)
		}
		d.db = db

		// Retry ping
		if err := d.ping(ctx); err != nil {
			return nil, fmt.Errorf("database connection failed after reconnect: %v", err)
		}

		// Re-detect edition after reconnect
		if err := d.detectDB2Edition(ctx); err != nil {
			d.Warningf("failed to detect DB2 edition after reconnect: %v", err)
		}
	}

	// Collect global metrics
	if err := d.collectGlobalMetrics(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect global metrics: %v", err)
	}

	// Collect per-instance metrics if enabled and supported
	if d.CollectDatabaseMetrics != nil && *d.CollectDatabaseMetrics {
		d.Debugf("collecting database instance metrics (limit: %d)", d.MaxDatabases)
		if err := d.collectDatabaseInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("database_instances_unavailable", "Database instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect database instances: %v", err)
			}
		}
	}

	if d.CollectBufferpoolMetrics != nil && *d.CollectBufferpoolMetrics {
		d.Debugf("collecting bufferpool instance metrics (limit: %d)", d.MaxBufferpools)
		if err := d.collectBufferpoolInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("bufferpool_instances_unavailable", "Bufferpool instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect bufferpool instances: %v", err)
			}
		}
	}

	if d.CollectTablespaceMetrics != nil && *d.CollectTablespaceMetrics {
		d.Debugf("collecting tablespace instance metrics (limit: %d)", d.MaxTablespaces)
		if err := d.collectTablespaceInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("tablespace_instances_unavailable", "Tablespace instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect tablespace instances: %v", err)
			}
		}
	}

	if d.CollectConnectionMetrics != nil && *d.CollectConnectionMetrics {
		d.Debugf("collecting connection instance metrics (limit: %d)", d.MaxConnections)
		if err := d.collectConnectionInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("connection_instances_unavailable", "Connection instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect connection instances: %v", err)
			}
		}
	}

	if d.CollectTableMetrics != nil && *d.CollectTableMetrics {
		d.Debugf("collecting table instance metrics (limit: %d)", d.MaxTables)
		if err := d.collectTableInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("table_instances_unavailable", "Table instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect table instances: %v", err)
			}
		}
	}

	if d.CollectIndexMetrics != nil && *d.CollectIndexMetrics {
		d.Debugf("collecting index instance metrics (limit: %d)", d.MaxIndexes)
		if err := d.collectIndexInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("index_instances_unavailable", "Index instance collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect index instances: %v", err)
			}
		}
	}

	// Collect new performance metrics
	if d.CollectStatementMetrics {
		d.Debugf("collecting statement cache metrics (limit: %d)", d.MaxStatements)
		if err := d.collectStatementInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("statement_cache_unavailable", "Statement cache collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect statement cache: %v", err)
			}
		}
	}

	if d.CollectMemoryMetrics {
		d.Debugf("collecting memory pool metrics")
		if err := d.collectMemoryPoolInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("memory_pool_unavailable", "Memory pool collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect memory pools: %v", err)
			}
		}

		// Screen 26: Collect Instance Memory Sets
		d.Debugf("collecting memory set instances (Screen 26)")
		if err := d.collectMemorySetInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("memory_set_unavailable", "Memory set collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect memory sets: %v", err)
			}
		}

		// Screen 15: Collect Prefetchers
		d.Debugf("collecting prefetcher instances (Screen 15)")
		if err := d.collectPrefetcherInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("prefetcher_unavailable", "Prefetcher collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect prefetchers: %v", err)
			}
		}
	}

	if d.CollectWaitMetrics && d.CollectConnectionMetrics != nil && *d.CollectConnectionMetrics {
		d.Debugf("collecting enhanced wait metrics")
		if err := d.collectConnectionWaits(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("wait_metrics_unavailable", "Wait metrics collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect wait metrics: %v", err)
			}
		}
	}

	if d.CollectTableIOMetrics {
		d.Debugf("collecting table I/O metrics")
		if err := d.collectTableIOInstances(ctx); err != nil {
			if isSQLFeatureError(err) {
				d.logOnce("table_io_unavailable", "Table I/O collection failed (likely unsupported on this DB2 edition/version): %v", err)
			} else {
				d.Errorf("failed to collect table I/O: %v", err)
			}
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

	for name, metrics := range d.mx.tables {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("table_%s_%s", cleanName, k)] = v
		}
	}

	for name, metrics := range d.mx.indexes {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("index_%s_%s", cleanName, k)] = v
		}
	}

	// Add new metric types
	for id, metrics := range d.mx.statements {
		// ID is already cleaned by cleanStmtID
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("statement_%s_%s", id, k)] = v
		}
	}

	for poolType, metrics := range d.mx.memoryPools {
		cleanType := cleanName(poolType)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("memory_pool_%s_%s", cleanType, k)] = v
		}
	}

	for tableName, metrics := range d.mx.tableIOs {
		cleanName := cleanName(tableName)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("table_io_%s_%s", cleanName, k)] = v
		}
	}

	// Add memory set metrics
	for setKey, metrics := range d.memorySets {
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
	for bufferPoolName, metrics := range d.prefetchers {
		cleanName := cleanName(bufferPoolName)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("prefetcher_%s_%s", cleanName, k)] = v
		}
	}

	return mx, nil
}

func (d *DB2) collectServiceHealth(ctx context.Context) {
	// Connection check
	d.mx.CanConnect = 0
	if err := d.doQuerySingleValue(ctx, queryCanConnect, &d.mx.CanConnect); err != nil {
		d.mx.CanConnect = 0
	}

	// Database status check
	// 0 = OK (active), 1 = WARNING (quiesce-pending, rollforward), 2 = CRITICAL (quiesced), 3 = UNKNOWN
	d.mx.DatabaseStatus = 3
	if err := d.doQuerySingleValue(ctx, queryDatabaseStatus, &d.mx.DatabaseStatus); err != nil {
		d.mx.DatabaseStatus = 3
	}
}

func (d *DB2) detectVersion(ctx context.Context) error {
	// Try SYSIBMADM.ENV_INST_INFO (works on LUW)
	query := queryDetectVersionLUW

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
	query = queryDetectVersionI
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
	d.Debugf("starting global metrics collection")

	// Service health checks - core functionality
	if err := d.collectServiceHealthResilience(ctx); err != nil {
		return err // Service health is critical
	}

	// Connection metrics - core functionality that should always work
	if err := d.collectConnectionMetricsResilience(ctx); err != nil {
		return err // Connection metrics are critical
	}

	// Database Overview metrics (Screen 01) - always collect if possible
	if err := d.collectDatabaseOverview(ctx); err != nil {
		d.Warningf("failed to collect database overview metrics: %v", err)
	}
	
	// Enhanced Logging Performance metrics (Screen 18)
	if err := d.collectLoggingPerformance(ctx); err != nil {
		d.Warningf("failed to collect enhanced logging performance metrics: %v", err)
	}
	
	// Federation metrics (Screen 32) - only if supported
	if err := d.collectFederationMetrics(ctx); err != nil {
		// Not logging as warning since federation might not be configured
		d.Debugf("federation metrics collection skipped: %v", err)
	}

	// Use modern MON_GET_* functions for better performance when available
	if d.useMonGetFunctions {
		// Collect all database-level metrics in one efficient call
		if !d.isDisabled("advanced_monitoring") {
			if err := d.collectMonGetDatabase(ctx); err != nil {
				d.Warningf("failed to collect database metrics using MON_GET_DATABASE: %v, falling back to individual queries", err)
				// Fall back to individual SNAP queries
				d.collectLockMetricsResilience(ctx)
				d.collectSortingMetricsResilience(ctx)
				d.collectRowActivityMetricsResilience(ctx)
			}
		} else {
			d.logOnce("advanced_monitoring_skipped", "Advanced monitoring metrics collection skipped - Not available on this DB2 edition/version")
		}

		// Buffer pool metrics using MON_GET_BUFFERPOOL
		if !d.isDisabled("bufferpool_detailed_metrics") {
			if err := d.collectMonGetBufferpoolAggregate(ctx); err != nil {
				d.Warningf("failed to collect bufferpool metrics using MON_GET_BUFFERPOOL: %v, falling back to SNAP views", err)
				d.collectBufferpoolMetricsResilience(ctx)
			}
		} else {
			d.logOnce("bufferpool_detailed_skipped", "Detailed buffer pool metrics collection skipped - Limited on this DB2 edition")
		}

		// Log space metrics using MON_GET_TRANSACTION_LOG
		if !d.isDisabled("system_level_metrics") {
			if err := d.collectMonGetTransactionLog(ctx); err != nil {
				d.Warningf("failed to collect log metrics using MON_GET_TRANSACTION_LOG: %v, falling back to SNAP views", err)
				d.collectLogSpaceMetricsResilience(ctx)
			}
		} else {
			d.logOnce("system_level_skipped", "System-level metrics collection skipped - Restricted on this DB2 edition")
		}
	} else {
		// Fall back to SNAP views approach
		// Advanced monitoring metrics - skip if disabled for this edition/version
		if !d.isDisabled("advanced_monitoring") {
			// Lock metrics - graceful degradation
			d.collectLockMetricsResilience(ctx)

			// Sorting metrics - graceful degradation
			d.collectSortingMetricsResilience(ctx)

			// Row activity metrics - graceful degradation
			d.collectRowActivityMetricsResilience(ctx)
		} else {
			d.logOnce("advanced_monitoring_skipped", "Advanced monitoring metrics collection skipped - Not available on this DB2 edition/version")
		}

		// Buffer pool metrics - may be limited on some editions
		if !d.isDisabled("bufferpool_detailed_metrics") {
			d.collectBufferpoolMetricsResilience(ctx)
		} else {
			d.logOnce("bufferpool_detailed_skipped", "Detailed buffer pool metrics collection skipped - Limited on this DB2 edition")
		}

		// Log space metrics - graceful degradation
		if !d.isDisabled("system_level_metrics") {
			d.collectLogSpaceMetricsResilience(ctx)
		} else {
			d.logOnce("system_level_skipped", "System-level metrics collection skipped - Restricted on this DB2 edition")
		}
	}

	// Long-running queries and backup status - collected separately as they don't have MON_GET equivalents
	if !d.isDisabled("advanced_monitoring") {
		// Long-running queries - graceful degradation
		d.collectLongRunningQueriesResilience(ctx)

		// Backup status - graceful degradation
		d.collectBackupStatusResilience(ctx)
	}

	d.Debugf("completed global metrics collection")
	return nil
}

func (d *DB2) collectLockMetrics(ctx context.Context) error {
	// Choose query based on monitoring approach
	var query string
	if d.useMonGetFunctions {
		// Use modern MON_GET_DATABASE for lock metrics
		query = queryMonGetDatabase
		d.Debugf("using MON_GET_DATABASE for lock metrics (modern approach)")
	} else if d.edition == "Cloud" {
		// Use Cloud-specific SNAP query
		query = queryLockMetricsCloud
		d.Debugf("using Cloud-specific lock metrics query for Db2 on Cloud")
	} else {
		// Use standard SNAP query
		query = queryLockMetrics
		d.Debugf("using SNAP views for lock metrics (legacy approach)")
	}

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
		case "LOCK_ACTIVE":
			d.mx.LockActive = v
		case "LOCK_WAIT_TIME":
			d.mx.LockWaitTime = v * Precision // Convert to milliseconds with Precision
		case "LOCK_WAITING_AGENTS":
			d.mx.LockWaitingAgents = v
		case "LOCK_MEMORY_PAGES":
			d.mx.LockMemoryPages = v
		case "TOTAL_SORTS":
			d.mx.TotalSorts = v
		case "SORT_OVERFLOWS":
			d.mx.SortOverflows = v
		case "ROWS_READ":
			d.mx.RowsRead = v
		case "ROWS_MODIFIED":
			d.mx.RowsModified = v
		case "ROWS_RETURNED":
			d.mx.RowsReturned = v
		}
	})
}

func (d *DB2) collectBufferpoolAggregateMetrics(ctx context.Context) error {
	// Choose query based on monitoring approach and column support
	var query string
	if d.useMonGetFunctions {
		query = queryMonGetBufferpoolAggregate
		d.Debugf("using MON_GET_BUFFERPOOL for aggregate metrics (modern approach)")

		// With MON_GET, we need to calculate hit ratios differently
		return d.collectMonGetBufferpoolAggregate(ctx)
	} else if d.supportsColumnOrganizedTables {
		query = queryBufferpoolAggregateMetrics
		d.Debugf("using SNAP views with column-organized metrics for bufferpool aggregate metrics")
	} else {
		query = queryBufferpoolAggregateMetricsLegacy
		d.Debugf("using legacy SNAP views without column-organized metrics for bufferpool aggregate metrics")
	}

	// Temporary variables to calculate hits and misses
	var dataHits, dataReads, indexHits, indexReads, xdaHits, xdaReads, colHits, colReads int64
	
	return d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "DATA_PAGES_FOUND":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				dataHits = v
			}
		case "DATA_L_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				dataReads = v
				// Calculate misses for data pages
				d.mx.BufferpoolDataHits = dataHits
				d.mx.BufferpoolDataMisses = dataReads - dataHits
				if dataReads > 0 {
					d.Debugf("Buffer pool data: hits=%d, reads=%d, misses=%d", dataHits, dataReads, dataReads-dataHits)
				}
			}
		case "INDEX_PAGES_FOUND":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				indexHits = v
			}
		case "INDEX_L_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				indexReads = v
				// Calculate misses for index pages
				d.mx.BufferpoolIndexHits = indexHits
				d.mx.BufferpoolIndexMisses = indexReads - indexHits
			}
		case "XDA_PAGES_FOUND":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				xdaHits = v
			}
		case "XDA_L_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				xdaReads = v
				// Calculate misses for XDA pages
				d.mx.BufferpoolXDAHits = xdaHits
				d.mx.BufferpoolXDAMisses = xdaReads - xdaHits
			}
		case "COL_PAGES_FOUND":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				colHits = v
			}
		case "COL_L_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				colReads = v
				// Calculate misses for column pages
				d.mx.BufferpoolColumnHits = colHits
				d.mx.BufferpoolColumnMisses = colReads - colHits
			}
		case "LOGICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolLogicalReads = v
			}
		case "PHYSICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolPhysicalReads = v
			}
		case "TOTAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolTotalReads = v
			}
		case "DATA_LOGICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolDataLogicalReads = v
			}
		case "DATA_PHYSICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolDataPhysicalReads = v
			}
		case "DATA_TOTAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolDataTotalReads = v
			}
		case "INDEX_LOGICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolIndexLogicalReads = v
			}
		case "INDEX_PHYSICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolIndexPhysicalReads = v
			}
		case "INDEX_TOTAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolIndexTotalReads = v
			}
		case "XDA_LOGICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolXDALogicalReads = v
			}
		case "XDA_PHYSICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolXDAPhysicalReads = v
			}
		case "XDA_TOTAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolXDATotalReads = v
			}
		case "COLUMN_LOGICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolColumnLogicalReads = v
			}
		case "COLUMN_PHYSICAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolColumnPhysicalReads = v
			}
		case "COLUMN_TOTAL_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.BufferpoolColumnTotalReads = v
			}
		}
		
		// Calculate overall hits and misses at the end of the row
		if lineEnd {
			// If misses are negative, it means prefetch brought more pages than were requested
			// In this case, set misses to 0 and reduce hits accordingly
			if d.mx.BufferpoolDataMisses < 0 {
				d.mx.BufferpoolDataHits = dataReads
				d.mx.BufferpoolDataMisses = 0
			}
			if d.mx.BufferpoolIndexMisses < 0 {
				d.mx.BufferpoolIndexHits = indexReads
				d.mx.BufferpoolIndexMisses = 0
			}
			if d.mx.BufferpoolXDAMisses < 0 {
				d.mx.BufferpoolXDAHits = xdaReads
				d.mx.BufferpoolXDAMisses = 0
			}
			if d.mx.BufferpoolColumnMisses < 0 {
				d.mx.BufferpoolColumnHits = colReads
				d.mx.BufferpoolColumnMisses = 0
			}
			
			// Overall buffer pool hits = sum of all hits
			d.mx.BufferpoolHits = d.mx.BufferpoolDataHits + d.mx.BufferpoolIndexHits + 
				d.mx.BufferpoolXDAHits + d.mx.BufferpoolColumnHits
			// Overall buffer pool misses = sum of all misses
			d.mx.BufferpoolMisses = d.mx.BufferpoolDataMisses + d.mx.BufferpoolIndexMisses + 
				d.mx.BufferpoolXDAMisses + d.mx.BufferpoolColumnMisses
		}
	})
}

func (d *DB2) collectLogSpaceMetrics(ctx context.Context) error {
	// Choose query based on monitoring approach
	var query string
	if d.useMonGetFunctions {
		query = queryMonGetTransactionLog
		d.Debugf("using MON_GET_TRANSACTION_LOG for log metrics (modern approach)")
	} else {
		query = queryLogSpaceMetrics
		d.Debugf("using SNAP views for log metrics (legacy approach)")
	}

	return d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_LOG_USED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogUsedSpace = v
			}
		case "TOTAL_LOG_AVAILABLE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogAvailableSpace = v
			}
		case "LOG_UTILIZATION":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				d.mx.LogUtilization = int64(v * Precision)
			}
		case "LOG_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogIOReads = v
			}
		case "LOG_WRITES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogIOWrites = v
			}
		}
	})
}

func (d *DB2) doQuery(ctx context.Context, query string, assign func(column, value string, lineEnd bool)) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	rows, err := d.db.QueryContext(queryCtx, query)
	if err != nil {
		if isSQLFeatureError(err) {
			d.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		} else {
			d.Errorf("failed to execute query: %s, error: %v", query, err)
		}
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
		if isSQLFeatureError(err) {
			d.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		}
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
		if isSQLFeatureError(err) {
			d.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		}
		return err
	}
	if value.Valid {
		*target = int64(value.Float64 * Precision)
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
			d.Debugf("ODBC driver panic in %s query (columns: %v): %v", currentQuery, columns, r)
			d.Debugf("This is a known ODBC driver limitation with certain DB2 data types")
		}
	}()

	for rows.Next() {
		// Wrap Scan in panic recovery as well since it can also trigger ODBC issues
		func() {
			defer func() {
				if r := recover(); r != nil {
					d.Debugf("ODBC scan panic recovered: %v", r)
				}
			}()
			
			if err := rows.Scan(valuePtrs...); err != nil {
				d.Debugf("Row scan error: %v", err)
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

func (d *DB2) collectLongRunningQueries(ctx context.Context) error {
	// Query to find long-running queries from SYSIBMADM.LONG_RUNNING_SQL
	// Warning threshold: 5 minutes, Critical threshold: 15 minutes
	return d.doQuery(ctx, queryLongRunningQueries, func(column, value string, lineEnd bool) {
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
	now := time.Now()

	// First check if we have ANY backup (successful or failed) in the last 7 days
	var lastBackupSQLCode sql.NullInt64
	var lastBackupTime sql.NullString
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout))
	defer cancel()

	// Get the most recent backup attempt with ODBC-safe error handling
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				d.Debugf("backup history query failed due to ODBC driver issue: %v", r)
				err = fmt.Errorf("odbc driver error: %v", r)
			}
		}()
		err = d.db.QueryRowContext(queryCtx, `
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
			d.mx.LastBackupStatus = 0 // Success
		} else {
			d.mx.LastBackupStatus = 1 // Failed (non-zero SQLCODE)
		}
	} else {
		// No backup history found
		d.mx.LastBackupStatus = 0 // Don't raise alert if no backup history
	}

	// Get the last successful full backup with ODBC-safe error handling
	var lastFullBackup sql.NullString
	func() {
		defer func() {
			if r := recover(); r != nil {
				d.Debugf("full backup query failed due to ODBC driver issue: %v", r)
				err = fmt.Errorf("odbc driver error: %v", r)
			}
		}()
		err = d.db.QueryRowContext(queryCtx, `
			SELECT MAX(START_TIME) 
			FROM SYSIBMADM.DB_HISTORY 
			WHERE OPERATION = 'B' 
			  AND OPERATIONTYPE = 'F' 
			  AND SQLCODE = 0
		`).Scan(&lastFullBackup)
	}()

	if err == nil && lastFullBackup.Valid {
		if t, err := time.Parse("2006-01-02-15.04.05", lastFullBackup.String); err == nil {
			d.mx.LastFullBackupAge = int64(now.Sub(t).Hours())
		} else {
			d.Warningf("failed to parse last full backup time '%s': %v (expected format: YYYY-MM-DD-HH.MM.SS)", lastFullBackup.String, err)
			d.mx.LastFullBackupAge = 0 // Parse error - report 0 hours (recent)
		}
	} else {
		// No successful backup found
		d.mx.LastFullBackupAge = 720 // 30 days in hours - old but reasonable
	}

	// Get the last successful incremental backup with ODBC-safe error handling
	var lastIncrementalBackup sql.NullString
	func() {
		defer func() {
			if r := recover(); r != nil {
				d.Debugf("incremental backup query failed due to ODBC driver issue: %v", r)
				err = fmt.Errorf("odbc driver error: %v", r)
			}
		}()
		err = d.db.QueryRowContext(queryCtx, `
			SELECT MAX(START_TIME) 
			FROM SYSIBMADM.DB_HISTORY 
			WHERE OPERATION = 'B' 
			  AND OPERATIONTYPE IN ('I', 'O', 'D') 
			  AND SQLCODE = 0
		`).Scan(&lastIncrementalBackup)
	}()

	if err == nil && lastIncrementalBackup.Valid {
		if t, err := time.Parse("2006-01-02-15.04.05", lastIncrementalBackup.String); err == nil {
			d.mx.LastIncrementalBackupAge = int64(now.Sub(t).Hours())
		} else {
			d.Warningf("failed to parse last incremental backup time '%s': %v (expected format: YYYY-MM-DD-HH.MM.SS)", lastIncrementalBackup.String, err)
			d.mx.LastIncrementalBackupAge = 0 // Parse error - report 0 hours (recent)
		}
	} else {
		// No incremental backup found - this is normal for many setups
		d.mx.LastIncrementalBackupAge = 0 // Report 0 to avoid false alerts
	}

	return nil
}

// Resilient collection functions following AS/400 pattern

func (d *DB2) collectServiceHealthResilience(ctx context.Context) error {
	// Service health is critical - these must work
	if err := d.collectSingleMetric(ctx, "can_connect", queryCanConnect, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.CanConnect = v
		}
	}); err != nil {
		return err // Fatal - basic connectivity must work
	}

	// Database status - optional on some editions
	_ = d.collectSingleMetric(ctx, "database_status", queryDatabaseStatus, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.DatabaseStatus = v
		}
	})

	return nil
}

func (d *DB2) collectConnectionMetricsResilience(ctx context.Context) error {
	// Use MON_GET if available for better performance
	if d.useMonGetFunctions {
		return d.collectMonGetConnections(ctx)
	}

	// Fall back to SNAP views
	// Core connection metrics - must work on all DB2 editions
	if err := d.collectSingleMetric(ctx, "total_connections", queryTotalConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.ConnTotal = v
		}
	}); err != nil {
		return err // Fatal - basic connection count must work
	}

	// Optional connection breakdowns - graceful degradation
	_ = d.collectSingleMetric(ctx, "active_connections", queryActiveConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.ConnActive = v
		}
	})

	_ = d.collectSingleMetric(ctx, "executing_connections", queryExecutingConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.ConnExecuting = v
		}
	})

	_ = d.collectSingleMetric(ctx, "idle_connections", queryIdleConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.ConnIdle = v
		}
	})

	_ = d.collectSingleMetric(ctx, "max_connections", queryMaxConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.ConnMax = v
		}
	})

	// Calculate idle if not available directly
	if d.mx.ConnIdle == 0 && d.mx.ConnActive > 0 {
		d.mx.ConnIdle = d.mx.ConnActive - d.mx.ConnExecuting
	}

	return nil
}

func (d *DB2) collectLockMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for lock metrics
	_ = d.collectSingleMetric(ctx, "lock_waits", queryLockWaits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockWaits = v
		}
	})

	_ = d.collectSingleMetric(ctx, "lock_timeouts", queryLockTimeouts, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockTimeouts = v
		}
	})

	_ = d.collectSingleMetric(ctx, "deadlocks", queryDeadlocks, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.Deadlocks = v
		}
	})

	_ = d.collectSingleMetric(ctx, "lock_escalations", queryLockEscalations, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockEscalations = v
		}
	})

	_ = d.collectSingleMetric(ctx, "active_locks", queryActiveLocks, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockActive = v
		}
	})

	_ = d.collectSingleMetric(ctx, "lock_wait_time", queryLockWaitTime, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockWaitTime = v * Precision // Convert to milliseconds with Precision
		}
	})

	_ = d.collectSingleMetric(ctx, "lock_waiting_agents", queryLockWaitingAgents, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockWaitingAgents = v
		}
	})

	_ = d.collectSingleMetric(ctx, "lock_memory_pages", queryLockMemoryPages, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LockMemoryPages = v
		}
	})
}

func (d *DB2) collectSortingMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for sorting metrics
	_ = d.collectSingleMetric(ctx, "total_sorts", queryTotalSorts, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.TotalSorts = v
		}
	})

	_ = d.collectSingleMetric(ctx, "sort_overflows", querySortOverflows, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.SortOverflows = v
		}
	})
}

func (d *DB2) collectRowActivityMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for row activity metrics
	_ = d.collectSingleMetric(ctx, "rows_read", queryRowsRead, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.RowsRead = v
		}
	})

	_ = d.collectSingleMetric(ctx, "rows_modified", queryRowsModified, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.RowsModified = v
		}
	})

	_ = d.collectSingleMetric(ctx, "rows_returned", queryRowsReturned, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.RowsReturned = v
		}
	})
}

func (d *DB2) collectBufferpoolMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for bufferpool metrics
	// Collect individual components first
	var dataLogical, dataHits int64
	var indexLogical, indexHits int64
	var xdaLogical, xdaHits int64
	var colLogical, colHits int64

	// Get reads
	_ = d.collectSingleMetric(ctx, "bufferpool_logical_reads", queryBufferpoolLogicalReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.BufferpoolLogicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_physical_reads", queryBufferpoolPhysicalReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.BufferpoolPhysicalReads = v
		}
	})

	// Data reads
	_ = d.collectSingleMetric(ctx, "bufferpool_data_logical", queryBufferpoolDataLogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			dataLogical = v
			d.mx.BufferpoolDataLogicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_data_physical", queryBufferpoolDataPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.BufferpoolDataPhysicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_data_hits", queryBufferpoolDataHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			dataHits = v
		}
	})

	// Index reads
	_ = d.collectSingleMetric(ctx, "bufferpool_index_logical", queryBufferpoolIndexLogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			indexLogical = v
			d.mx.BufferpoolIndexLogicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_index_physical", queryBufferpoolIndexPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.BufferpoolIndexPhysicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_index_hits", queryBufferpoolIndexHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			indexHits = v
		}
	})

	// XDA reads
	_ = d.collectSingleMetric(ctx, "bufferpool_xda_logical", queryBufferpoolXDALogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			xdaLogical = v
			d.mx.BufferpoolXDALogicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_xda_physical", queryBufferpoolXDAPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.BufferpoolXDAPhysicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_xda_hits", queryBufferpoolXDAHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			xdaHits = v
		}
	})

	// Column reads
	_ = d.collectSingleMetric(ctx, "bufferpool_column_logical", queryBufferpoolColumnLogical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			colLogical = v
			d.mx.BufferpoolColumnLogicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_column_physical", queryBufferpoolColumnPhysical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.BufferpoolColumnPhysicalReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "bufferpool_column_hits", queryBufferpoolColumnHits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			colHits = v
		}
	})

	// Calculate hit ratios from components
	// Calculate hits and misses for each type
	d.mx.BufferpoolDataHits = dataHits
	d.mx.BufferpoolDataMisses = dataLogical - dataHits
	
	d.mx.BufferpoolIndexHits = indexHits
	d.mx.BufferpoolIndexMisses = indexLogical - indexHits
	
	d.mx.BufferpoolXDAHits = xdaHits
	d.mx.BufferpoolXDAMisses = xdaLogical - xdaHits
	
	d.mx.BufferpoolColumnHits = colHits
	d.mx.BufferpoolColumnMisses = colLogical - colHits
	
	// If misses are negative, it means prefetch brought more pages than were requested
	// In this case, set misses to 0 and reduce hits accordingly
	if d.mx.BufferpoolDataMisses < 0 {
		d.mx.BufferpoolDataHits = dataLogical
		d.mx.BufferpoolDataMisses = 0
	}
	if d.mx.BufferpoolIndexMisses < 0 {
		d.mx.BufferpoolIndexHits = indexLogical
		d.mx.BufferpoolIndexMisses = 0
	}
	if d.mx.BufferpoolXDAMisses < 0 {
		d.mx.BufferpoolXDAHits = xdaLogical
		d.mx.BufferpoolXDAMisses = 0
	}
	if d.mx.BufferpoolColumnMisses < 0 {
		d.mx.BufferpoolColumnHits = colLogical
		d.mx.BufferpoolColumnMisses = 0
	}
	
	// Calculate overall hits and misses
	d.mx.BufferpoolHits = d.mx.BufferpoolDataHits + d.mx.BufferpoolIndexHits + 
		d.mx.BufferpoolXDAHits + d.mx.BufferpoolColumnHits
	d.mx.BufferpoolMisses = d.mx.BufferpoolDataMisses + d.mx.BufferpoolIndexMisses + 
		d.mx.BufferpoolXDAMisses + d.mx.BufferpoolColumnMisses
}

func (d *DB2) collectLogSpaceMetricsResilience(ctx context.Context) {
	// Individual SNAP queries for log space metrics
	var logUsed, logAvailable int64

	_ = d.collectSingleMetric(ctx, "log_used_space", queryLogUsedSpace, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			logUsed = v
			d.mx.LogUsedSpace = v
		}
	})

	_ = d.collectSingleMetric(ctx, "log_available_space", queryLogAvailableSpace, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			logAvailable = v
			d.mx.LogAvailableSpace = v
		}
	})

	_ = d.collectSingleMetric(ctx, "log_reads", queryLogReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LogIOReads = v
		}
	})

	_ = d.collectSingleMetric(ctx, "log_writes", queryLogWrites, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LogIOWrites = v
		}
	})

	// Calculate log utilization
	if logUsed > 0 && logAvailable > 0 {
		total := logUsed + logAvailable
		d.mx.LogUtilization = int64((float64(logUsed) * 100.0 * float64(Precision)) / float64(total))
	}
}

func (d *DB2) collectLongRunningQueriesResilience(ctx context.Context) {
	_ = d.collectSingleMetric(ctx, "long_running_total", queryLongRunningTotal, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LongRunningQueries = v
		}
	})

	_ = d.collectSingleMetric(ctx, "long_running_warning", queryLongRunningWarning, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LongRunningQueriesWarning = v
		}
	})

	_ = d.collectSingleMetric(ctx, "long_running_critical", queryLongRunningCritical, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LongRunningQueriesCritical = v
		}
	})
}

func (d *DB2) collectBackupStatusResilience(ctx context.Context) {
	// This will use the existing collectBackupStatus function as it's already resilient
	_ = d.collectBackupStatus(ctx)
}

// MON_GET collection functions for modern monitoring approach

func (d *DB2) collectMonGetConnections(ctx context.Context) error {
	// First get connection counts
	err := d.doQuery(ctx, queryMonGetConnections, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnTotal = v
			}
		case "ACTIVE_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnActive = v
			}
		case "IDLE_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnIdle = v
			}
		case "EXECUTING_CONNS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnExecuting = v
			}
		}
	})

	if err != nil {
		return err
	}

	// Then get max connections from configuration
	return d.collectSingleMetric(ctx, "max_connections", queryMaxConnections, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.ConnMax = v
		}
	})
}

func (d *DB2) collectMonGetDatabase(ctx context.Context) error {
	return d.doQuery(ctx, queryMonGetDatabase, func(column, value string, lineEnd bool) {
		switch column {
		case "LOCK_WAITS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockWaits = v
			}
		case "LOCK_TIMEOUTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockTimeouts = v
			}
		case "DEADLOCKS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.Deadlocks = v
			}
		case "LOCK_ESCALS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockEscalations = v
			}
		case "LOCK_ACTIVE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockActive = v
			}
		case "LOCK_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockWaitTime = v
			}
		case "LOCK_WAITING_AGENTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockWaitingAgents = v
			}
		case "LOCK_MEMORY_PAGES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockMemoryPages = v
			}
		case "TOTAL_SORTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.TotalSorts = v
			}
		case "SORT_OVERFLOWS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.SortOverflows = v
			}
		case "ROWS_READ":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.RowsRead = v
			}
		case "ROWS_MODIFIED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.RowsModified = v
			}
		case "ROWS_RETURNED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.RowsReturned = v
			}
		}
	})
}

func (d *DB2) collectMonGetBufferpoolAggregate(ctx context.Context) error {
	var dataLogical, dataPhysical, dataHits int64
	var indexLogical, indexPhysical, indexHits int64
	var xdaLogical, xdaPhysical, xdaHits int64
	var colLogical, colPhysical, colHits int64

	err := d.doQuery(ctx, queryMonGetBufferpoolAggregate, func(column, value string, lineEnd bool) {
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return
		}

		switch column {
		case "DATA_LOGICAL_READS":
			dataLogical = v
			d.mx.BufferpoolDataLogicalReads = v
		case "DATA_PHYSICAL_READS":
			dataPhysical = v
			d.mx.BufferpoolDataPhysicalReads = v
		case "DATA_HITS":
			dataHits = v
		case "INDEX_LOGICAL_READS":
			indexLogical = v
			d.mx.BufferpoolIndexLogicalReads = v
		case "INDEX_PHYSICAL_READS":
			indexPhysical = v
			d.mx.BufferpoolIndexPhysicalReads = v
		case "INDEX_HITS":
			indexHits = v
		case "XDA_LOGICAL_READS":
			xdaLogical = v
			d.mx.BufferpoolXDALogicalReads = v
		case "XDA_PHYSICAL_READS":
			xdaPhysical = v
			d.mx.BufferpoolXDAPhysicalReads = v
		case "XDA_HITS":
			xdaHits = v
		case "COLUMN_LOGICAL_READS":
			colLogical = v
			d.mx.BufferpoolColumnLogicalReads = v
		case "COLUMN_PHYSICAL_READS":
			colPhysical = v
			d.mx.BufferpoolColumnPhysicalReads = v
		case "COLUMN_HITS":
			colHits = v
		}
	})

	if err != nil {
		return err
	}

	// Calculate totals
	d.mx.BufferpoolLogicalReads = dataLogical + indexLogical + xdaLogical + colLogical
	d.mx.BufferpoolPhysicalReads = dataPhysical + indexPhysical + xdaPhysical + colPhysical
	d.mx.BufferpoolTotalReads = d.mx.BufferpoolLogicalReads + d.mx.BufferpoolPhysicalReads

	// Calculate data totals
	d.mx.BufferpoolDataTotalReads = dataLogical + dataPhysical
	d.mx.BufferpoolIndexTotalReads = indexLogical + indexPhysical
	d.mx.BufferpoolXDATotalReads = xdaLogical + xdaPhysical
	d.mx.BufferpoolColumnTotalReads = colLogical + colPhysical

	// Calculate hits and misses for each type
	d.mx.BufferpoolDataHits = dataHits
	d.mx.BufferpoolDataMisses = dataLogical - dataHits
	
	d.mx.BufferpoolIndexHits = indexHits
	d.mx.BufferpoolIndexMisses = indexLogical - indexHits
	
	d.mx.BufferpoolXDAHits = xdaHits
	d.mx.BufferpoolXDAMisses = xdaLogical - xdaHits
	
	d.mx.BufferpoolColumnHits = colHits
	d.mx.BufferpoolColumnMisses = colLogical - colHits
	
	// If misses are negative, it means prefetch brought more pages than were requested
	// In this case, set misses to 0 and reduce hits accordingly
	if d.mx.BufferpoolDataMisses < 0 {
		d.mx.BufferpoolDataHits = dataLogical
		d.mx.BufferpoolDataMisses = 0
	}
	if d.mx.BufferpoolIndexMisses < 0 {
		d.mx.BufferpoolIndexHits = indexLogical
		d.mx.BufferpoolIndexMisses = 0
	}
	if d.mx.BufferpoolXDAMisses < 0 {
		d.mx.BufferpoolXDAHits = xdaLogical
		d.mx.BufferpoolXDAMisses = 0
	}
	if d.mx.BufferpoolColumnMisses < 0 {
		d.mx.BufferpoolColumnHits = colLogical
		d.mx.BufferpoolColumnMisses = 0
	}
	
	// Calculate overall hits and misses
	d.mx.BufferpoolHits = d.mx.BufferpoolDataHits + d.mx.BufferpoolIndexHits + 
		d.mx.BufferpoolXDAHits + d.mx.BufferpoolColumnHits
	d.mx.BufferpoolMisses = d.mx.BufferpoolDataMisses + d.mx.BufferpoolIndexMisses + 
		d.mx.BufferpoolXDAMisses + d.mx.BufferpoolColumnMisses

	return nil
}

func (d *DB2) collectMonGetTransactionLog(ctx context.Context) error {
	return d.doQuery(ctx, queryMonGetTransactionLog, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_LOG_USED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogUsedSpace = v
			}
		case "TOTAL_LOG_AVAILABLE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogAvailableSpace = v
			}
		case "LOG_UTILIZATION":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				d.mx.LogUtilization = int64(v * Precision)
			}
		case "LOG_READS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogIOReads = v
			}
		case "LOG_WRITES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogIOWrites = v
			}
		}
	})
}

// Database Overview collection (Screen 01)
func (d *DB2) collectDatabaseOverview(ctx context.Context) error {
	// Use simple queries approach (based on dcmtop) for more resilience
	// Run multiple simple queries instead of one complex query
	
	// Helper function to run a simple query and handle errors gracefully
	// It tries the MON_GET query first, then falls back to SNAP query if that fails
	runSimpleQuery := func(queryMonGet, querySnap string, handler func(column, value string)) {
		var err error
		if d.useMonGetFunctions && queryMonGet != "" {
			err = d.doQuery(ctx, queryMonGet, func(column, value string, lineEnd bool) {
				handler(column, value)
			})
			if err != nil && isSQLFeatureError(err) {
				d.Debugf("MON_GET query not supported, trying SNAP query: %v", err)
				if querySnap != "" {
					err = d.doQuery(ctx, querySnap, func(column, value string, lineEnd bool) {
						handler(column, value)
					})
				}
			}
		} else if querySnap != "" {
			err = d.doQuery(ctx, querySnap, func(column, value string, lineEnd bool) {
				handler(column, value)
			})
		}
		if err != nil {
			d.Debugf("query failed (will continue): %v", err)
		}
	}
	
	// Database status
	runSimpleQuery(querySimpleDatabaseStatus, querySnapDatabaseStatus, func(column, value string) {
		if column == "DATABASE_STATUS" && value == "ACTIVE" {
			d.mx.DatabaseActive = 1
			d.mx.DatabaseInactive = 0
		} else {
			d.mx.DatabaseActive = 0
			d.mx.DatabaseInactive = 1
		}
	})
	
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
		d.mx.CPUUser = int64((cpuUserNs / totalNs) * 100 * Precision)
		d.mx.CPUSystem = int64((cpuSystemNs / totalNs) * 100 * Precision)
		d.mx.CPUIdle = int64((cpuIdleNs / totalNs) * 100 * Precision)
		d.mx.CPUIowait = int64((cpuIowaitNs / totalNs) * 100 * Precision)
	}
	
	// Connection metrics
	runSimpleQuery(querySimpleConnectionsActive, querySnapConnectionsActive, func(column, value string) {
		if column == "ACTIVE_CONNECTIONS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnectionsActive = v
			}
		}
	})
	
	runSimpleQuery(querySimpleConnectionsTotal, querySnapConnectionsTotal, func(column, value string) {
		if column == "TOTAL_CONNECTIONS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.ConnectionsTotal = v
			}
		}
	})
	
	// Memory metrics
	runSimpleQuery(querySimpleMemoryInstance, "", func(column, value string) {
		if column == "INSTANCE_MEM_COMMITTED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.MemoryInstanceCommitted = v
			}
		}
	})
	
	runSimpleQuery(querySimpleMemoryDatabase, "", func(column, value string) {
		if column == "DATABASE_MEM_COMMITTED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.MemoryDatabaseCommitted = v
			}
		}
	})
	
	runSimpleQuery(querySimpleMemoryBufferpool, "", func(column, value string) {
		if column == "BUFFERPOOL_MEM_USED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.MemoryBufferpoolUsed = v
			}
		}
	})
	
	runSimpleQuery(querySimpleMemorySharedSort, "", func(column, value string) {
		if column == "SHARED_SORT_MEM_USED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.MemorySharedSortUsed = v
			}
		}
	})
	
	// Throughput metrics
	runSimpleQuery(querySimpleTransactions, querySnapTransactions, func(column, value string) {
		if column == "TRANSACTIONS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.OpsTransactions = v
			}
		}
	})
	
	runSimpleQuery(querySimpleSelectStmts, querySnapSelectStmts, func(column, value string) {
		if column == "SELECT_STMTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.OpsSelectStmts = v
			}
		}
	})
	
	runSimpleQuery(querySimpleUIDStmts, querySnapUIDStmts, func(column, value string) {
		if column == "UID_STMTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.OpsUIDStmts = v
			}
		}
	})
	
	runSimpleQuery(querySimpleActivitiesAborted, "", func(column, value string) {
		if column == "ACTIVITIES_ABORTED" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.OpsActivitiesAborted = v
			}
		}
	})
	
	// Time spent metrics
	runSimpleQuery(querySimpleAvgDirectReadTime, querySnapAvgDirectReadTime, func(column, value string) {
		if column == "AVG_DIRECT_READ_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				d.mx.TimeAvgDirectRead = int64(v * Precision * 1000) // Convert to microseconds
			}
		}
	})
	
	runSimpleQuery(querySimpleAvgDirectWriteTime, querySnapAvgDirectWriteTime, func(column, value string) {
		if column == "AVG_DIRECT_WRITE_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				d.mx.TimeAvgDirectWrite = int64(v * Precision * 1000)
			}
		}
	})
	
	runSimpleQuery(querySimpleAvgPoolReadTime, querySnapAvgPoolReadTime, func(column, value string) {
		if column == "AVG_POOL_READ_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				d.mx.TimeAvgPoolRead = int64(v * Precision * 1000)
			}
		}
	})
	
	runSimpleQuery(querySimpleAvgPoolWriteTime, querySnapAvgPoolWriteTime, func(column, value string) {
		if column == "AVG_POOL_WRITE_TIME" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				d.mx.TimeAvgPoolWrite = int64(v * Precision * 1000)
			}
		}
	})
	
	// Additional metrics that were not in the original complex query
	// but are collected by dcmtop
	
	// Lock metrics
	runSimpleQuery(querySimpleLockHeld, querySnapLockHeld, func(column, value string) {
		if column == "LOCKS_HELD" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockActive = v
			}
		}
	})
	
	runSimpleQuery(querySimpleLockWaits, querySnapLockWaits, func(column, value string) {
		if column == "LOCK_WAITS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockWaits = v
			}
		}
	})
	
	runSimpleQuery(querySimpleLockTimeouts, querySnapLockTimeouts, func(column, value string) {
		if column == "LOCK_TIMEOUTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LockTimeouts = v
			}
		}
	})
	
	runSimpleQuery(querySimpleDeadlocks, querySnapDeadlocks, func(column, value string) {
		if column == "DEADLOCKS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.Deadlocks = v
			}
		}
	})
	
	// Log operations
	runSimpleQuery(querySimpleLogReads, querySnapLogReads, func(column, value string) {
		if column == "LOG_READS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogOpReads = v
			}
		}
	})
	
	runSimpleQuery(querySimpleLogWrites, querySnapLogWrites, func(column, value string) {
		if column == "LOG_WRITES" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.LogOpWrites = v
			}
		}
	})
	
	// Sorts
	runSimpleQuery(querySimpleSorts, querySnapSorts, func(column, value string) {
		if column == "SORTS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.TotalSorts = v
			}
		}
	})
	
	runSimpleQuery(querySimpleSortOverflows, querySnapSortOverflows, func(column, value string) {
		if column == "SORT_OVERFLOWS" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.SortOverflows = v
			}
		}
	})
	
	return nil // Always return nil since we run queries individually and handle errors gracefully
}

// Enhanced Logging Performance collection (Screen 18)
func (d *DB2) collectLoggingPerformance(ctx context.Context) error {
	// Use individual tested queries from dcmtop instead of complex combined queries
	
	// Collect log commits (from TOTAL_APP_COMMITS)
	_ = d.collectSingleMetric(ctx, "log_commits", queryLogCommits, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LogCommits = v
		}
	})
	
	// Collect log rollbacks (from TOTAL_APP_ROLLBACKS)
	_ = d.collectSingleMetric(ctx, "log_rollbacks", queryLogRollbacks, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LogRollbacks = v
		}
	})
	
	// Collect log I/O reads (from NUM_LOG_READ_IO)
	_ = d.collectSingleMetric(ctx, "log_io_reads", queryLoggingReads, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LogIOReads = v
		}
	})
	
	// Collect log I/O writes (from NUM_LOG_WRITE_IO)
	_ = d.collectSingleMetric(ctx, "log_io_writes", queryLoggingWrites, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			d.mx.LogIOWrites = v
		}
	})
	
	return nil
}

// Federation metrics collection (Screen 32)
func (d *DB2) collectFederationMetrics(ctx context.Context) error {
	// Skip if federation is not supported
	if !d.supportsFederation {
		return nil
	}
	
	query := queryFederationMetrics
	if !d.supportsExtendedMetrics {
		query = queryFederationMetricsSimple
	}
	
	return d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "FED_CONNECTIONS_ACTIVE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.FedConnectionsActive = v
			}
		case "FED_CONNECTIONS_IDLE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.FedConnectionsIdle = v
			}
		case "FED_ROWS_READ":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.FedRowsRead = v
			}
		case "FED_SELECT_STMTS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.FedSelectStmts = v
			}
		case "FED_WAITS_TOTAL":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.mx.FedWaitsTotal = v
			}
		}
	})
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
