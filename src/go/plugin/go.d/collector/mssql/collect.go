// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth/sqladapter"
)

// noLatencySentinel is the value SQL Server returns when no latency data is available
const noLatencySentinel = 999999

const (
	maxKBToBytes        = int64(1<<63-1) / 1024
	maxKBToCentiPercent = int64(1<<63-1) / 10000
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.db == nil {
		db, err := c.openConnection()
		if err != nil {
			return nil, err
		}
		c.db = db
	}

	if c.version == "" {
		ver, err := c.queryVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to query version: %v", err)
		}
		c.version = ver
		c.majorVersion = parseMajorVersion(c.version)
		c.Debugf("connected to SQL Server version %s (major: %d)", c.version, c.majorVersion)
	}

	if !c.hadrChecked {
		if err := c.checkHadrEnabled(); err != nil {
			c.Debugf("HADR check failed: %v", err)
		}
		c.hadrChecked = true
	}

	mx := make(map[string]int64)

	if err := c.collectInstanceMetrics(mx); err != nil {
		return nil, err
	}
	if err := c.collectDatabaseMetrics(mx); err != nil {
		return nil, err
	}
	if err := c.collectLockMetrics(mx); err != nil {
		return nil, err
	}
	if err := c.collectWaitStats(mx); err != nil {
		return nil, err
	}
	if err := c.collectJobStatus(mx); err != nil {
		return nil, err
	}
	if err := c.collectReplicationStatus(mx); err != nil {
		return nil, err
	}
	if c.hadrEnabled {
		if err := c.collectAvailabilityGroups(mx); err != nil {
			c.Warningf("AG metrics collection failed: %v", err)
		}
	}

	return mx, nil
}

func (c *Collector) openConnection() (*sql.DB, error) {
	driverName, dsn, err := c.resolveConnectionParams()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening connection: %v", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	return db, nil
}

func (c *Collector) resolveConnectionParams() (string, string, error) {
	driverName := sqladapter.MSSQLDriver(c.CloudAuth)
	dsn := c.DSN
	if c.CloudAuth.IsEnabled() {
		var err error
		dsn, err = sqladapter.BuildMSSQLAzureADDSN(c.DSN, c.CloudAuth)
		if err != nil {
			return "", "", fmt.Errorf("error preparing cloud auth SQL Server DSN: %v", err)
		}
	}
	return driverName, dsn, nil
}

func (c *Collector) queryVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var version string
	err := c.db.QueryRowContext(ctx, queryVersion).Scan(&version)
	if err != nil {
		return "", err
	}

	return version, nil
}

func (c *Collector) collectInstanceMetrics(mx map[string]int64) error {
	if err := c.collectUserConnections(mx); err != nil {
		return err
	}
	if err := c.collectBlockedProcesses(mx); err != nil {
		return err
	}
	if err := c.collectBatchRequests(mx); err != nil {
		return err
	}
	if err := c.collectCompilations(mx); err != nil {
		return err
	}
	if err := c.collectSQLErrors(mx); err != nil {
		return err
	}
	if err := c.collectBufferManager(mx); err != nil {
		return err
	}
	if err := c.collectMemoryManager(mx); err != nil {
		return err
	}
	if err := c.collectAccessMethods(mx); err != nil {
		return err
	}
	// Process and OS memory metrics (always collected)
	if err := c.collectProcessMemory(mx); err != nil {
		return err
	}
	if err := c.collectOSMemory(mx); err != nil {
		return err
	}

	return nil
}

func (c *Collector) collectUserConnections(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var userConns, sysConns int64
	err := c.db.QueryRowContext(ctx, queryUserConnections).Scan(&userConns, &sysConns)
	if err != nil {
		return fmt.Errorf("user connections query failed: %v", err)
	}

	mx["user_connections"] = userConns
	// Session connections: user vs internal (system)
	mx["session_connections_user"] = userConns
	mx["session_connections_internal"] = sysConns

	return nil
}

func (c *Collector) collectBlockedProcesses(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var blocked int64
	err := c.db.QueryRowContext(ctx, queryBlockedProcesses).Scan(&blocked)
	if err != nil {
		return fmt.Errorf("blocked processes query failed: %v", err)
	}

	mx["blocked_processes"] = blocked

	return nil
}

func (c *Collector) collectBatchRequests(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var value int64
	err := c.db.QueryRowContext(ctx, queryBatchRequests).Scan(&value)
	if err != nil {
		return fmt.Errorf("batch requests query failed: %v", err)
	}

	mx["batch_requests"] = value

	return nil
}

func (c *Collector) collectCompilations(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryCompilations)
	if err != nil {
		return fmt.Errorf("compilations query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var counterName string
		var value int64
		if err := rows.Scan(&counterName, &value); err != nil {
			continue
		}

		counterName = strings.TrimSpace(counterName)
		switch counterName {
		case "SQL Compilations/sec":
			mx["sql_compilations"] = value
		case "SQL Re-Compilations/sec":
			mx["sql_recompilations"] = value
		case "Auto-Param Attempts/sec":
			mx["auto_param_attempts"] = value
		case "Safe Auto-Params/sec":
			mx["auto_param_safe"] = value
		case "Failed Auto-Params/sec":
			mx["auto_param_failed"] = value
		}
	}

	return rows.Err()
}

func (c *Collector) collectBufferManager(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryBufferManager)
	if err != nil {
		return fmt.Errorf("buffer manager query failed: %v", err)
	}
	defer rows.Close()

	var cacheHitRatio, cacheHitRatioBase int64

	for rows.Next() {
		var counterName string
		var value int64
		if err := rows.Scan(&counterName, &value); err != nil {
			continue
		}

		counterName = strings.TrimSpace(counterName)
		switch counterName {
		case "Page reads/sec":
			mx["buffer_page_reads"] = value
		case "Page writes/sec":
			mx["buffer_page_writes"] = value
		case "Buffer cache hit ratio":
			cacheHitRatio = value
		case "Buffer cache hit ratio base":
			cacheHitRatioBase = value
		case "Checkpoint pages/sec":
			mx["buffer_checkpoint_pages"] = value
		case "Page life expectancy":
			mx["buffer_page_life_expectancy"] = value
		case "Lazy writes/sec":
			mx["buffer_lazy_writes"] = value
		case "Page lookups/sec":
			mx["buffer_page_lookups"] = value
		}
	}

	// Calculate hit ratio as percentage
	if cacheHitRatioBase > 0 {
		mx["buffer_cache_hit_ratio"] = (cacheHitRatio * 100) / cacheHitRatioBase
	}

	return rows.Err()
}

func (c *Collector) collectMemoryManager(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryMemoryManager)
	if err != nil {
		return fmt.Errorf("memory manager query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var counterName string
		var value int64
		if err := rows.Scan(&counterName, &value); err != nil {
			continue
		}

		counterName = strings.TrimSpace(counterName)
		switch counterName {
		case "Total Server Memory (KB)":
			mx["memory_total"] = value * 1024 // Convert to bytes
		case "Connection Memory (KB)":
			mx["memory_connection"] = value * 1024
		case "Memory Grants Pending":
			mx["memory_pending_grants"] = value
		case "External benefit of memory":
			mx["memory_external_benefit"] = value
		}
	}

	return rows.Err()
}

func (c *Collector) collectAccessMethods(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var value int64
	err := c.db.QueryRowContext(ctx, queryAccessMethods).Scan(&value)
	if err != nil {
		return fmt.Errorf("access methods query failed: %v", err)
	}

	mx["page_splits"] = value

	return nil
}

func (c *Collector) collectDatabaseMetrics(mx map[string]int64) error {
	if err := c.collectDatabaseCounters(mx); err != nil {
		return err
	}
	if err := c.collectLockStatsByResourceType(mx); err != nil {
		return err
	}
	if err := c.collectDatabaseSize(mx); err != nil {
		return err
	}
	if err := c.collectDatabaseStatus(mx); err != nil {
		return err
	}
	// Collect I/O stall and log growth metrics per database
	if err := c.collectIOStall(mx); err != nil {
		return err
	}
	if err := c.collectLogGrowths(mx); err != nil {
		return err
	}
	if err := c.collectDatabaseLogCounters(mx); err != nil {
		return err
	}

	return nil
}

func (c *Collector) collectDatabaseCounters(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryDatabaseCounters)
	if err != nil {
		return fmt.Errorf("database counters query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName, counterName string
		var value int64
		if err := rows.Scan(&dbName, &counterName, &value); err != nil {
			continue
		}

		dbName = strings.TrimSpace(dbName)
		counterName = strings.TrimSpace(counterName)

		if !c.seenDatabases[dbName] {
			c.seenDatabases[dbName] = true
			c.addDatabaseCharts(dbName)
		}

		dbID := cleanDatabaseName(dbName)
		switch counterName {
		case "Active Transactions":
			mx[fmt.Sprintf("database_%s_active_transactions", dbID)] = value
		case "Transactions/sec":
			mx[fmt.Sprintf("database_%s_transactions", dbID)] = value
		case "Write Transactions/sec":
			mx[fmt.Sprintf("database_%s_write_transactions", dbID)] = value
		case "Backup/Restore Throughput/sec":
			mx[fmt.Sprintf("database_%s_backup_restore_throughput", dbID)] = value
		case "Log Bytes Flushed/sec":
			mx[fmt.Sprintf("database_%s_log_flushed", dbID)] = value
		case "Log Flushes/sec":
			mx[fmt.Sprintf("database_%s_log_flushes", dbID)] = value
		}
	}

	return rows.Err()
}

type databaseLogCounters struct {
	sizeKB          int64
	usedKB          int64
	truncations     int64
	shrinks         int64
	hasSize         bool
	hasUsed         bool
	hasTruncations  bool
	hasShrinks      bool
	hasKnownCounter bool
}

func (c *Collector) collectDatabaseLogCounters(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryDatabaseLogCounters)
	if err != nil {
		return fmt.Errorf("database log counters query failed: %v", err)
	}
	defer rows.Close()

	counters := make(map[string]*databaseLogCounters)

	for rows.Next() {
		var dbName, counterName string
		var value int64
		if err := rows.Scan(&dbName, &counterName, &value); err != nil {
			continue
		}
		if value < 0 {
			continue
		}

		dbName = strings.TrimSpace(dbName)
		counterName = strings.TrimSpace(counterName)
		if dbName == "" {
			continue
		}

		if !c.seenDatabasesWithLog[dbName] {
			c.seenDatabasesWithLog[dbName] = true
			c.addDatabaseLogCharts(dbName)
		}

		if counters[dbName] == nil {
			counters[dbName] = &databaseLogCounters{}
		}

		switch counterName {
		case "Log File(s) Size (KB)":
			counters[dbName].sizeKB = value
			counters[dbName].hasSize = true
			counters[dbName].hasKnownCounter = true
		case "Log File(s) Used Size (KB)":
			counters[dbName].usedKB = value
			counters[dbName].hasUsed = true
			counters[dbName].hasKnownCounter = true
		case "Log Truncations":
			counters[dbName].truncations = value
			counters[dbName].hasTruncations = true
			counters[dbName].hasKnownCounter = true
		case "Log Shrinks":
			counters[dbName].shrinks = value
			counters[dbName].hasShrinks = true
			counters[dbName].hasKnownCounter = true
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	for dbName, values := range counters {
		if values == nil || !values.hasKnownCounter {
			continue
		}

		dbID := cleanDatabaseName(dbName)

		if values.hasSize && values.hasUsed && values.sizeKB > 0 &&
			values.sizeKB <= maxKBToBytes && values.usedKB <= maxKBToBytes {
			usedBytes := values.usedKB * 1024
			freeKB := max(values.sizeKB-values.usedKB, 0)

			mx[fmt.Sprintf("database_%s_log_size_used", dbID)] = usedBytes
			mx[fmt.Sprintf("database_%s_log_size_free", dbID)] = freeKB * 1024

			if values.usedKB <= maxKBToCentiPercent {
				mx[fmt.Sprintf("database_%s_log_percent_used", dbID)] = values.usedKB * 10000 / values.sizeKB
			}
		}

		if values.hasTruncations {
			mx[fmt.Sprintf("database_%s_log_truncations", dbID)] = values.truncations
		}
		if values.hasShrinks {
			mx[fmt.Sprintf("database_%s_log_shrinks", dbID)] = values.shrinks
		}
	}

	return nil
}

func (c *Collector) collectLockStatsByResourceType(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryDatabaseLocks)
	if err != nil {
		return fmt.Errorf("lock stats query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType, counterName string
		var value int64
		if err := rows.Scan(&resourceType, &counterName, &value); err != nil {
			continue
		}

		resourceType = strings.TrimSpace(resourceType)
		counterName = strings.TrimSpace(counterName)

		if !c.seenLockStatsTypes[resourceType] {
			c.seenLockStatsTypes[resourceType] = true
			c.addLockStatsCharts(resourceType)
		}

		// Note: instance_name from Locks counter is the lock resource type, not database name
		resID := cleanResourceTypeName(resourceType)
		switch counterName {
		case "Number of Deadlocks/sec":
			mx[fmt.Sprintf("lock_stats_%s_deadlocks", resID)] = value
		case "Lock Waits/sec":
			mx[fmt.Sprintf("lock_stats_%s_waits", resID)] = value
		case "Lock Timeouts/sec":
			mx[fmt.Sprintf("lock_stats_%s_timeouts", resID)] = value
		case "Lock Requests/sec":
			mx[fmt.Sprintf("lock_stats_%s_requests", resID)] = value
		}
	}

	return rows.Err()
}

func (c *Collector) collectDatabaseSize(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryDatabaseSize)
	if err != nil {
		return fmt.Errorf("database size query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		var size int64
		if err := rows.Scan(&dbName, &size); err != nil {
			continue
		}

		dbName = strings.TrimSpace(dbName)

		if !c.seenDatabases[dbName] {
			c.seenDatabases[dbName] = true
			c.addDatabaseCharts(dbName)
		}

		dbID := cleanDatabaseName(dbName)
		mx[fmt.Sprintf("database_%s_data_file_size", dbID)] = size
	}

	return rows.Err()
}

func (c *Collector) collectLockMetrics(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryLocksByResource)
	if err != nil {
		return fmt.Errorf("locks by resource query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType string
		var count int64
		if err := rows.Scan(&resourceType, &count); err != nil {
			continue
		}

		resourceType = strings.TrimSpace(resourceType)

		if !c.seenLockTypes[resourceType] {
			c.seenLockTypes[resourceType] = true
			c.addLockResourceCharts(resourceType)
		}

		resID := cleanResourceTypeName(resourceType)
		mx[fmt.Sprintf("locks_%s_count", resID)] = count
	}

	return rows.Err()
}

func (c *Collector) collectWaitStats(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryWaitStats)
	if err != nil {
		return fmt.Errorf("wait stats query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var waitType string
		var totalWait, resourceWait, signalWait, maxWait, waitingTasks int64
		if err := rows.Scan(&waitType, &totalWait, &resourceWait, &signalWait, &maxWait, &waitingTasks); err != nil {
			continue
		}

		waitType = strings.TrimSpace(waitType)
		waitCategory := getWaitCategory(waitType)

		if !c.seenWaitTypes[waitType] {
			c.seenWaitTypes[waitType] = true
			c.addWaitTypeCharts(waitType, waitCategory)
		}

		waitID := cleanWaitTypeName(waitType)
		mx[fmt.Sprintf("wait_%s_total_ms", waitID)] = totalWait
		mx[fmt.Sprintf("wait_%s_resource_ms", waitID)] = resourceWait
		mx[fmt.Sprintf("wait_%s_signal_ms", waitID)] = signalWait
		mx[fmt.Sprintf("wait_%s_max_ms", waitID)] = maxWait
		mx[fmt.Sprintf("wait_%s_tasks", waitID)] = waitingTasks
	}

	return rows.Err()
}

func (c *Collector) collectJobStatus(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryJobs)
	if err != nil {
		return fmt.Errorf("jobs query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jobName string
		var enabled int64
		if err := rows.Scan(&jobName, &enabled); err != nil {
			continue
		}

		jobName = strings.TrimSpace(jobName)

		if !c.seenJobs[jobName] {
			c.seenJobs[jobName] = true
			c.addJobCharts(jobName)
		}

		jobID := cleanJobName(jobName)
		if enabled == 1 {
			mx[fmt.Sprintf("job_%s_enabled", jobID)] = 1
			mx[fmt.Sprintf("job_%s_disabled", jobID)] = 0
		} else {
			mx[fmt.Sprintf("job_%s_enabled", jobID)] = 0
			mx[fmt.Sprintf("job_%s_disabled", jobID)] = 1
		}
	}

	return rows.Err()
}

func (c *Collector) collectSQLErrors(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var value int64
	err := c.db.QueryRowContext(ctx, querySQLErrors).Scan(&value)
	if err != nil {
		return fmt.Errorf("sql errors query failed: %v", err)
	}

	mx["sql_errors_total"] = value

	return nil
}

func (c *Collector) collectDatabaseStatus(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryDatabaseStatus)
	if err != nil {
		return fmt.Errorf("database status query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		var state int64
		var isReadOnly bool
		if err := rows.Scan(&dbName, &state, &isReadOnly); err != nil {
			continue
		}

		dbName = strings.TrimSpace(dbName)

		if !c.seenDatabases[dbName] {
			c.seenDatabases[dbName] = true
			c.addDatabaseCharts(dbName)
		}

		dbID := cleanDatabaseName(dbName)

		// Database state values:
		// 0 = ONLINE, 1 = RESTORING, 2 = RECOVERING, 3 = RECOVERY_PENDING
		// 4 = SUSPECT, 5 = EMERGENCY, 6 = OFFLINE
		mx[fmt.Sprintf("database_%s_state_online", dbID)] = boolToInt(state == 0)
		mx[fmt.Sprintf("database_%s_state_restoring", dbID)] = boolToInt(state == 1)
		mx[fmt.Sprintf("database_%s_state_recovering", dbID)] = boolToInt(state == 2)
		mx[fmt.Sprintf("database_%s_state_pending", dbID)] = boolToInt(state == 3)
		mx[fmt.Sprintf("database_%s_state_suspect", dbID)] = boolToInt(state == 4)
		mx[fmt.Sprintf("database_%s_state_emergency", dbID)] = boolToInt(state == 5)
		mx[fmt.Sprintf("database_%s_state_offline", dbID)] = boolToInt(state == 6)

		// Read-only status
		mx[fmt.Sprintf("database_%s_read_only", dbID)] = boolToInt(isReadOnly)
		mx[fmt.Sprintf("database_%s_read_write", dbID)] = boolToInt(!isReadOnly)
	}

	return rows.Err()
}

func (c *Collector) collectReplicationStatus(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	// First collect monitor data (status, latency, etc.)
	rows, err := c.db.QueryContext(ctx, queryReplicationStatus)
	if err != nil {
		// Replication may not be configured, don't treat as error
		c.Debugf("replication status query failed (may not be configured): %v", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var pubDB, publication string
		var status, warning, worstLatency, bestLatency, avgLatency, runningAgents int64
		if err := rows.Scan(&pubDB, &publication, &status, &warning, &worstLatency, &bestLatency, &avgLatency, &runningAgents); err != nil {
			c.Debugf("replication scan error: %v", err)
			continue
		}

		pubDB = strings.TrimSpace(pubDB)
		publication = strings.TrimSpace(publication)

		pubKey := pubDB + "_" + publication
		if !c.seenReplications[pubKey] {
			c.seenReplications[pubKey] = true
			c.addReplicationCharts(pubDB, publication)
		}

		pubID := cleanPublicationName(pubDB, publication)

		// Decode status into 6 discrete states (matching C implementation)
		// 1=started, 2=succeeded, 3=in_progress, 4=idle, 5=retrying, 6=failed
		mx[fmt.Sprintf("replication_%s_status_started", pubID)] = boolToInt(status == 1)
		mx[fmt.Sprintf("replication_%s_status_succeeded", pubID)] = boolToInt(status == 2)
		mx[fmt.Sprintf("replication_%s_status_in_progress", pubID)] = boolToInt(status == 3)
		mx[fmt.Sprintf("replication_%s_status_idle", pubID)] = boolToInt(status == 4)
		mx[fmt.Sprintf("replication_%s_status_retrying", pubID)] = boolToInt(status == 5)
		mx[fmt.Sprintf("replication_%s_status_failed", pubID)] = boolToInt(status == 6)

		// Decode warning into 7 individual flags (bitfield)
		// Bit 0x01: expiration, 0x02: latency, 0x04: mergeexpiration
		// 0x08: mergeslowrunduration, 0x10: mergefastrunduration
		// 0x20: mergefastrunspeed, 0x40: mergeslowrunspeed
		mx[fmt.Sprintf("replication_%s_warning_expiration", pubID)] = boolToInt(warning&0x01 != 0)
		mx[fmt.Sprintf("replication_%s_warning_latency", pubID)] = boolToInt(warning&0x02 != 0)
		mx[fmt.Sprintf("replication_%s_warning_mergeexpiration", pubID)] = boolToInt(warning&0x04 != 0)
		mx[fmt.Sprintf("replication_%s_warning_mergeslowrunduration", pubID)] = boolToInt(warning&0x08 != 0)
		mx[fmt.Sprintf("replication_%s_warning_mergefastrunduration", pubID)] = boolToInt(warning&0x10 != 0)
		mx[fmt.Sprintf("replication_%s_warning_mergefastrunspeed", pubID)] = boolToInt(warning&0x20 != 0)
		mx[fmt.Sprintf("replication_%s_warning_mergeslowrunspeed", pubID)] = boolToInt(warning&0x40 != 0)

		mx[fmt.Sprintf("replication_%s_latency_avg", pubID)] = avgLatency
		// Handle the noLatencySentinel for "no value" (only bestLatency uses sentinel in query)
		if bestLatency == noLatencySentinel {
			bestLatency = 0
		}
		mx[fmt.Sprintf("replication_%s_latency_best", pubID)] = bestLatency
		mx[fmt.Sprintf("replication_%s_latency_worst", pubID)] = worstLatency
		mx[fmt.Sprintf("replication_%s_agents_running", pubID)] = runningAgents
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// Now collect subscription counts from MSpublications/MSsubscriptions
	rows2, err := c.db.QueryContext(ctx, querySubscriptionCount)
	if err != nil {
		c.Debugf("subscription count query failed: %v", err)
		return nil
	}
	defer rows2.Close()

	for rows2.Next() {
		var pubDB, publication string
		var subCount int64
		if err := rows2.Scan(&pubDB, &publication, &subCount); err != nil {
			continue
		}

		pubDB = strings.TrimSpace(pubDB)
		publication = strings.TrimSpace(publication)
		pubID := cleanPublicationName(pubDB, publication)
		mx[fmt.Sprintf("replication_%s_subscriptions", pubID)] = subCount
	}

	return rows2.Err()
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// parseMajorVersion extracts the major version from a string like "16.0.4175.1"
func parseMajorVersion(version string) int {
	parts := strings.SplitN(version, ".", 2)
	if len(parts) == 0 {
		return 0
	}
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return v
}

func (c *Collector) checkHadrEnabled() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var enabled sql.NullInt64
	err := c.db.QueryRowContext(ctx, queryHadrEnabled).Scan(&enabled)
	if err != nil {
		return fmt.Errorf("HADR enabled check failed: %v", err)
	}

	c.hadrEnabled = enabled.Valid && enabled.Int64 == 1

	if c.hadrEnabled {
		c.Debugf("Always On Availability Groups is enabled")
	} else {
		c.Debugf("Always On Availability Groups is not enabled, skipping AG metrics")
	}

	return nil
}

// agDatabaseReplicaQuery returns the appropriate query for the SQL Server version
func agDatabaseReplicaQuery(majorVersion int) string {
	if majorVersion >= 13 { // SQL Server 2016+
		return queryAGDatabaseReplicas16
	}
	return queryAGDatabaseReplicasPre16 // SQL Server 2012-2014
}

func (c *Collector) collectAvailabilityGroups(mx map[string]int64) error {
	if err := c.collectAGHealth(mx); err != nil {
		return err
	}
	if err := c.collectAGReplicaStates(mx); err != nil {
		return err
	}
	if err := c.collectAGDatabaseReplicas(mx); err != nil {
		return err
	}
	if err := c.collectAGCluster(mx); err != nil {
		c.Debugf("AG cluster query failed (WSFC may not be configured): %v", err)
	}
	if err := c.collectAGClusterMembers(mx); err != nil {
		c.Debugf("AG cluster members query failed: %v", err)
	}
	if err := c.collectAGFailoverReadiness(mx); err != nil {
		c.Debugf("AG failover readiness query failed: %v", err)
	}
	if err := c.collectAGAutoPageRepair(mx); err != nil {
		c.Debugf("AG auto page repair query failed: %v", err)
	}
	if c.majorVersion >= 15 { // SQL Server 2019+
		if err := c.collectAGThreads(mx); err != nil {
			c.Debugf("AG threads query failed: %v", err)
		}
	}
	return nil
}

func (c *Collector) collectAGHealth(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryAGHealth)
	if err != nil {
		return fmt.Errorf("AG health query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agName string
		var syncHealth, primaryRecoveryHealth, secondaryRecoveryHealth int64
		if err := rows.Scan(&agName, &syncHealth, &primaryRecoveryHealth, &secondaryRecoveryHealth); err != nil {
			c.Debugf("AG health scan failed: %v", err)
			continue
		}

		agName = strings.TrimSpace(agName)

		if !c.seenAGs[agName] {
			c.seenAGs[agName] = true
			c.addAGCharts(agName)
		}

		agID := cleanAGName(agName)

		// sync health: 0=not_healthy, 1=partially_healthy, 2=healthy
		mx[fmt.Sprintf("ag_%s_sync_health_not_healthy", agID)] = boolToInt(syncHealth == 0)
		mx[fmt.Sprintf("ag_%s_sync_health_partially_healthy", agID)] = boolToInt(syncHealth == 1)
		mx[fmt.Sprintf("ag_%s_sync_health_healthy", agID)] = boolToInt(syncHealth == 2)

		// primary recovery health: -1=N/A (on secondary), 0=in_progress, 1=online
		mx[fmt.Sprintf("ag_%s_primary_recovery_online", agID)] = boolToInt(primaryRecoveryHealth == 1)
		mx[fmt.Sprintf("ag_%s_primary_recovery_in_progress", agID)] = boolToInt(primaryRecoveryHealth == 0)

		// secondary recovery health: -1=N/A (on primary), 0=in_progress, 1=online
		mx[fmt.Sprintf("ag_%s_secondary_recovery_online", agID)] = boolToInt(secondaryRecoveryHealth == 1)
		mx[fmt.Sprintf("ag_%s_secondary_recovery_in_progress", agID)] = boolToInt(secondaryRecoveryHealth == 0)
	}

	return rows.Err()
}

func (c *Collector) collectAGReplicaStates(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryAGReplicaStates)
	if err != nil {
		return fmt.Errorf("AG replica states query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agName, replicaServer, availMode, failoverMode string
		var role, connState, syncHealth int64
		if err := rows.Scan(&agName, &replicaServer, &availMode, &failoverMode,
			&role, &connState, &syncHealth); err != nil {
			c.Debugf("AG replica state scan failed: %v", err)
			continue
		}

		agName = strings.TrimSpace(agName)
		replicaServer = strings.TrimSpace(replicaServer)

		replicaKey := agName + "_" + replicaServer
		if !c.seenAGReplicas[replicaKey] {
			c.seenAGReplicas[replicaKey] = true
			c.addAGReplicaCharts(agName, replicaServer, availMode, failoverMode)
		}

		rID := cleanAGReplicaName(agName, replicaServer)

		// role: -1=unknown, 0=resolving, 1=primary, 2=secondary
		mx[fmt.Sprintf("ag_replica_%s_role_resolving", rID)] = boolToInt(role == 0)
		mx[fmt.Sprintf("ag_replica_%s_role_primary", rID)] = boolToInt(role == 1)
		mx[fmt.Sprintf("ag_replica_%s_role_secondary", rID)] = boolToInt(role == 2)
		mx[fmt.Sprintf("ag_replica_%s_role_unknown", rID)] = boolToInt(role == -1)

		// connected state: -1=unknown, 0=disconnected, 1=connected
		mx[fmt.Sprintf("ag_replica_%s_connected", rID)] = boolToInt(connState == 1)
		mx[fmt.Sprintf("ag_replica_%s_disconnected", rID)] = boolToInt(connState == 0)
		mx[fmt.Sprintf("ag_replica_%s_conn_unknown", rID)] = boolToInt(connState == -1)

		// sync health: 0=not_healthy, 1=partially_healthy, 2=healthy
		mx[fmt.Sprintf("ag_replica_%s_sync_health_not_healthy", rID)] = boolToInt(syncHealth == 0)
		mx[fmt.Sprintf("ag_replica_%s_sync_health_partially_healthy", rID)] = boolToInt(syncHealth == 1)
		mx[fmt.Sprintf("ag_replica_%s_sync_health_healthy", rID)] = boolToInt(syncHealth == 2)
	}

	return rows.Err()
}

func (c *Collector) collectAGDatabaseReplicas(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	query := agDatabaseReplicaQuery(c.majorVersion)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("AG database replicas query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agName, replicaServer, dbName string
		var syncState, isSuspended int64
		var logSendQueue, logSendRate, redoQueue, redoRate, filestreamRate int64
		var secondaryLag int64

		if err := rows.Scan(
			&agName, &replicaServer, &dbName,
			&syncState, &isSuspended,
			&logSendQueue, &logSendRate,
			&redoQueue, &redoRate, &filestreamRate,
			&secondaryLag,
		); err != nil {
			c.Debugf("AG database replica scan failed: %v", err)
			continue
		}

		agName = strings.TrimSpace(agName)
		replicaServer = strings.TrimSpace(replicaServer)
		dbName = strings.TrimSpace(dbName)

		if dbName == "" {
			continue
		}

		drKey := agName + "_" + replicaServer + "_" + dbName
		if !c.seenAGDatabaseReplicas[drKey] {
			c.seenAGDatabaseReplicas[drKey] = true
			c.addAGDatabaseReplicaCharts(agName, replicaServer, dbName)
		}

		drID := cleanAGDatabaseReplicaName(agName, replicaServer, dbName)

		// sync state: 0=not_synchronizing, 1=synchronizing, 2=synchronized, 3=reverting, 4=initializing
		mx[fmt.Sprintf("ag_db_%s_sync_state_not_synchronizing", drID)] = boolToInt(syncState == 0)
		mx[fmt.Sprintf("ag_db_%s_sync_state_synchronizing", drID)] = boolToInt(syncState == 1)
		mx[fmt.Sprintf("ag_db_%s_sync_state_synchronized", drID)] = boolToInt(syncState == 2)
		mx[fmt.Sprintf("ag_db_%s_sync_state_reverting", drID)] = boolToInt(syncState == 3)
		mx[fmt.Sprintf("ag_db_%s_sync_state_initializing", drID)] = boolToInt(syncState == 4)

		// queue sizes and rates (already converted to bytes in SQL)
		mx[fmt.Sprintf("ag_db_%s_log_send_queue_size", drID)] = logSendQueue
		mx[fmt.Sprintf("ag_db_%s_log_send_rate", drID)] = logSendRate
		mx[fmt.Sprintf("ag_db_%s_redo_queue_size", drID)] = redoQueue
		mx[fmt.Sprintf("ag_db_%s_redo_rate", drID)] = redoRate
		mx[fmt.Sprintf("ag_db_%s_filestream_send_rate", drID)] = filestreamRate

		// suspended
		mx[fmt.Sprintf("ag_db_%s_suspended", drID)] = isSuspended
		mx[fmt.Sprintf("ag_db_%s_not_suspended", drID)] = boolToInt(isSuspended == 0)

		// secondary lag (only meaningful on SQL 2016+, -1 = not available)
		if secondaryLag >= 0 {
			mx[fmt.Sprintf("ag_db_%s_secondary_lag_seconds", drID)] = secondaryLag
		}
	}

	return rows.Err()
}

func (c *Collector) collectAGCluster(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var quorumState int64
	err := c.db.QueryRowContext(ctx, queryAGCluster).Scan(&quorumState)
	if err != nil {
		return fmt.Errorf("AG cluster query failed: %v", err)
	}

	if !c.agClusterChartAdded {
		c.agClusterChartAdded = true
		if err := c.Charts().Add(agClusterQuorumStateChart.Copy()); err != nil {
			c.Warning(err)
		}
	}

	// quorum state: 0=unknown, 1=normal, 2=forced
	mx["ag_cluster_quorum_state_unknown"] = boolToInt(quorumState == 0)
	mx["ag_cluster_quorum_state_normal"] = boolToInt(quorumState == 1)
	mx["ag_cluster_quorum_state_forced"] = boolToInt(quorumState == 2)

	return nil
}

func (c *Collector) collectAGClusterMembers(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryAGClusterMembers)
	if err != nil {
		return fmt.Errorf("AG cluster members query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var memberName string
		var memberState, quorumVotes int64
		if err := rows.Scan(&memberName, &memberState, &quorumVotes); err != nil {
			c.Debugf("AG cluster member scan failed: %v", err)
			continue
		}

		memberName = strings.TrimSpace(memberName)

		if !c.seenAGClusterMembers[memberName] {
			c.seenAGClusterMembers[memberName] = true
			c.addAGClusterMemberCharts(memberName)
		}

		mID := cleanAGName(memberName)

		// member state: 0=offline, 1=online
		mx[fmt.Sprintf("ag_cluster_member_%s_up", mID)] = boolToInt(memberState == 1)
		mx[fmt.Sprintf("ag_cluster_member_%s_down", mID)] = boolToInt(memberState == 0)
		mx[fmt.Sprintf("ag_cluster_member_%s_quorum_votes", mID)] = quorumVotes
	}

	return rows.Err()
}

func (c *Collector) collectAGFailoverReadiness(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryAGFailoverReadiness)
	if err != nil {
		return fmt.Errorf("AG failover readiness query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agName, replicaServer, dbName string
		var isFailoverReady, isDatabaseJoined int64
		if err := rows.Scan(&agName, &replicaServer, &dbName, &isFailoverReady, &isDatabaseJoined); err != nil {
			c.Debugf("AG failover readiness scan failed: %v", err)
			continue
		}

		agName = strings.TrimSpace(agName)
		replicaServer = strings.TrimSpace(replicaServer)
		dbName = strings.TrimSpace(dbName)

		drID := cleanAGDatabaseReplicaName(agName, replicaServer, dbName)

		mx[fmt.Sprintf("ag_db_%s_failover_ready", drID)] = isFailoverReady
		mx[fmt.Sprintf("ag_db_%s_failover_not_ready", drID)] = boolToInt(isFailoverReady == 0)
		mx[fmt.Sprintf("ag_db_%s_joined", drID)] = isDatabaseJoined
		mx[fmt.Sprintf("ag_db_%s_not_joined", drID)] = boolToInt(isDatabaseJoined == 0)
	}

	return rows.Err()
}

func (c *Collector) collectAGAutoPageRepair(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryAGAutoPageRepair)
	if err != nil {
		return fmt.Errorf("AG auto page repair query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		var successfulRepairs, failedRepairs int64
		if err := rows.Scan(&dbName, &successfulRepairs, &failedRepairs); err != nil {
			c.Debugf("AG page repair scan failed: %v", err)
			continue
		}

		dbName = strings.TrimSpace(dbName)

		if dbName == "" {
			continue
		}

		if !c.seenAGPageRepairDBs[dbName] {
			c.seenAGPageRepairDBs[dbName] = true
			c.addAGPageRepairCharts(dbName)
		}

		dbID := cleanDatabaseName(dbName)

		mx[fmt.Sprintf("ag_page_repair_%s_successful", dbID)] = successfulRepairs
		mx[fmt.Sprintf("ag_page_repair_%s_failed", dbID)] = failedRepairs
	}

	return rows.Err()
}

func (c *Collector) collectAGThreads(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryAGThreads)
	if err != nil {
		return fmt.Errorf("AG threads query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agName string
		var captureThreads, redoThreads, parallelRedoThreads int64
		if err := rows.Scan(&agName, &captureThreads, &redoThreads, &parallelRedoThreads); err != nil {
			c.Debugf("AG threads scan failed: %v", err)
			continue
		}

		agID := cleanAGName(strings.TrimSpace(agName))

		mx[fmt.Sprintf("ag_%s_capture_threads", agID)] = captureThreads
		mx[fmt.Sprintf("ag_%s_redo_threads", agID)] = redoThreads
		mx[fmt.Sprintf("ag_%s_parallel_redo_threads", agID)] = parallelRedoThreads
	}

	return rows.Err()
}

func (c *Collector) collectProcessMemory(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var resident, virtual, utilization, pageFaults int64
	err := c.db.QueryRowContext(ctx, queryProcessMemory).Scan(&resident, &virtual, &utilization, &pageFaults)
	if err != nil {
		return fmt.Errorf("process memory query failed: %v", err)
	}

	mx["process_memory_resident"] = resident
	mx["process_memory_virtual"] = virtual
	mx["process_memory_utilization"] = utilization
	mx["process_page_faults"] = pageFaults

	return nil
}

func (c *Collector) collectOSMemory(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	var memUsed, memAvailable, pagefileUsed, pagefileAvailable int64
	err := c.db.QueryRowContext(ctx, queryOSMemory).Scan(&memUsed, &memAvailable, &pagefileUsed, &pagefileAvailable)
	if err != nil {
		return fmt.Errorf("OS memory query failed: %v", err)
	}

	mx["os_memory_used"] = memUsed
	mx["os_memory_available"] = memAvailable
	mx["os_pagefile_used"] = pagefileUsed
	mx["os_pagefile_available"] = pagefileAvailable

	return nil
}

func (c *Collector) collectIOStall(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryIOStall)
	if err != nil {
		return fmt.Errorf("IO stall query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		var readMs, writeMs, totalMs int64
		if err := rows.Scan(&dbName, &readMs, &writeMs, &totalMs); err != nil {
			continue
		}

		dbName = strings.TrimSpace(dbName)
		if dbName == "" {
			continue
		}

		if !c.seenDatabases[dbName] {
			c.seenDatabases[dbName] = true
			c.addDatabaseCharts(dbName)
		}

		dbID := cleanDatabaseName(dbName)
		mx[fmt.Sprintf("database_%s_io_stall_read", dbID)] = readMs
		mx[fmt.Sprintf("database_%s_io_stall_write", dbID)] = writeMs
	}

	return rows.Err()
}

func (c *Collector) collectLogGrowths(mx map[string]int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, queryLogGrowths)
	if err != nil {
		return fmt.Errorf("log growths query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		var growths int64
		if err := rows.Scan(&dbName, &growths); err != nil {
			continue
		}

		dbName = strings.TrimSpace(dbName)
		if dbName == "" {
			continue
		}

		if !c.seenDatabases[dbName] {
			c.seenDatabases[dbName] = true
			c.addDatabaseCharts(dbName)
		}

		dbID := cleanDatabaseName(dbName)
		mx[fmt.Sprintf("database_%s_log_growths", dbID)] = growths
	}

	return rows.Err()
}
