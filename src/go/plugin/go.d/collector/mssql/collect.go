// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
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
		c.Debugf("connected to SQL Server version %s", c.version)
	}

	mx := make(map[string]int64)

	if err := c.collectInstanceMetrics(mx); err != nil {
		return nil, err
	}

	if c.CollectTransactions {
		if err := c.collectDatabaseMetrics(mx); err != nil {
			c.Warning(err)
		}
	}

	if c.CollectLocks {
		if err := c.collectLockMetrics(mx); err != nil {
			c.Warning(err)
		}
	}

	if c.CollectWaits {
		if err := c.collectWaitStats(mx); err != nil {
			c.Warning(err)
		}
	}

	if c.CollectJobs {
		if err := c.collectJobStatus(mx); err != nil {
			c.Warning(err)
		}
	}

	if c.CollectReplication {
		if err := c.collectReplicationStatus(mx); err != nil {
			c.Warning(err)
		}
	}

	return mx, nil
}

func (c *Collector) openConnection() (*sql.DB, error) {
	db, err := sql.Open("sqlserver", c.DSN)
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
	if c.CollectUserConnections {
		if err := c.collectUserConnections(mx); err != nil {
			c.Warning(err)
		}
	}

	if c.CollectBlockedProcesses {
		if err := c.collectBlockedProcesses(mx); err != nil {
			c.Warning(err)
		}
	}

	if err := c.collectBatchRequests(mx); err != nil {
		c.Warning(err)
	}

	if err := c.collectCompilations(mx); err != nil {
		c.Warning(err)
	}

	if c.CollectSQLErrors {
		if err := c.collectSQLErrors(mx); err != nil {
			c.Warning(err)
		}
	}

	if c.CollectBufferStats {
		if err := c.collectBufferManager(mx); err != nil {
			c.Warning(err)
		}
		if err := c.collectMemoryManager(mx); err != nil {
			c.Warning(err)
		}
		if err := c.collectAccessMethods(mx); err != nil {
			c.Warning(err)
		}
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
		c.Warning(err)
	}
	if c.CollectDatabaseSize {
		if err := c.collectDatabaseSize(mx); err != nil {
			c.Warning(err)
		}
	}
	if c.CollectDatabaseStatus {
		if err := c.collectDatabaseStatus(mx); err != nil {
			c.Warning(err)
		}
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
			// Currently not charted
		case "Log Bytes Flushed/sec":
			mx[fmt.Sprintf("database_%s_log_flushed", dbID)] = value
		case "Log Flushes/sec":
			mx[fmt.Sprintf("database_%s_log_flushes", dbID)] = value
		}
	}

	return rows.Err()
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
		mx[fmt.Sprintf("replication_%s_status", pubID)] = status
		mx[fmt.Sprintf("replication_%s_warning", pubID)] = warning
		mx[fmt.Sprintf("replication_%s_latency_avg", pubID)] = avgLatency
		// Handle the 999999 sentinel for "no value"
		if bestLatency == 999999 {
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
