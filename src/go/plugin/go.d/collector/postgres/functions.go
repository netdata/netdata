// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"fmt"
)

// detectPgStatStatementsColumns queries the database to find available columns
func (c *Collector) detectPgStatStatementsColumns(ctx context.Context) (map[string]bool, error) {
	// Fast path: return cached result
	c.pgStatStatementsMu.RLock()
	if c.pgStatStatementsColumns != nil {
		cols := c.pgStatStatementsColumns
		c.pgStatStatementsMu.RUnlock()
		return cols, nil
	}
	c.pgStatStatementsMu.RUnlock()

	// Slow path: query and cache
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock
	if c.pgStatStatementsColumns != nil {
		return c.pgStatStatementsColumns, nil
	}

	// Query available columns from pg_stat_statements
	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'pg_stat_statements'
		AND table_schema = 'public'
	`
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, fmt.Errorf("failed to scan column name: %v", err)
		}
		cols[colName] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %v", err)
	}

	// Cache the result
	c.pgStatStatementsColumns = cols
	return cols, nil
}

// checkPgStatStatements checks if pg_stat_statements extension is available
// Only positive results are cached - negative results are re-checked each time
// so users don't need to restart after installing the extension
func (c *Collector) checkPgStatStatements(ctx context.Context) (bool, error) {
	// Fast path: return cached positive result
	c.pgStatStatementsMu.RLock()
	avail := c.pgStatStatementsAvail
	c.pgStatStatementsMu.RUnlock()
	if avail {
		return true, nil
	}

	// Slow path: query the database
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`
	err := c.db.QueryRowContext(ctx, query).Scan(&exists)
	if err != nil {
		return false, err
	}

	// Only cache positive results
	if exists {
		c.pgStatStatementsMu.Lock()
		c.pgStatStatementsAvail = true
		c.pgStatStatementsMu.Unlock()
	}

	return exists, nil
}

// checkPgStatMonitor checks if pg_stat_monitor extension is available
// Only positive results are cached - negative results are re-checked each time
func (c *Collector) checkPgStatMonitor(ctx context.Context) (bool, error) {
	// Fast path: return cached positive result
	c.pgStatStatementsMu.RLock()
	avail := c.pgStatMonitorAvail
	c.pgStatStatementsMu.RUnlock()
	if avail {
		return true, nil
	}

	// Slow path: query the database
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_monitor')`
	err := c.db.QueryRowContext(ctx, query).Scan(&exists)
	if err != nil {
		return false, err
	}

	// Only cache positive results
	if exists {
		c.pgStatStatementsMu.Lock()
		c.pgStatMonitorAvail = true
		c.pgStatStatementsMu.Unlock()
	}

	return exists, nil
}

// detectPgStatMonitorColumns queries the database to find available columns
func (c *Collector) detectPgStatMonitorColumns(ctx context.Context) (map[string]bool, error) {
	// Fast path: return cached result
	c.pgStatStatementsMu.RLock()
	if c.pgStatMonitorColumns != nil {
		cols := c.pgStatMonitorColumns
		c.pgStatStatementsMu.RUnlock()
		return cols, nil
	}
	c.pgStatStatementsMu.RUnlock()

	// Slow path: query and cache
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock
	if c.pgStatMonitorColumns != nil {
		return c.pgStatMonitorColumns, nil
	}

	// Query available columns from pg_stat_monitor
	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'pg_stat_monitor'
		AND table_schema = 'public'
	`
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, fmt.Errorf("failed to scan column name: %v", err)
		}
		cols[colName] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %v", err)
	}

	// Cache the result
	c.pgStatMonitorColumns = cols
	return cols, nil
}

// queryStatsSourceName is the type for query stats source
type queryStatsSourceName string

const (
	queryStatsSourcePgStatMonitor    queryStatsSourceName = "pg_stat_monitor"
	queryStatsSourcePgStatStatements queryStatsSourceName = "pg_stat_statements"
	queryStatsSourceNone             queryStatsSourceName = ""
)

// getQueryStatsSource detects and returns the best available query stats source.
// Prefers pg_stat_monitor if available, falls back to pg_stat_statements.
// Result is cached after first detection.
func (c *Collector) getQueryStatsSource(ctx context.Context) (queryStatsSourceName, error) {
	// Fast path: return cached result
	c.pgStatStatementsMu.RLock()
	source := c.queryStatsSource
	c.pgStatStatementsMu.RUnlock()
	if source != "" {
		return queryStatsSourceName(source), nil
	}

	// Slow path: detect and cache
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock
	if c.queryStatsSource != "" {
		return queryStatsSourceName(c.queryStatsSource), nil
	}

	// Check pg_stat_monitor first (preferred)
	var hasPgStatMonitor bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_monitor')`
	if err := c.db.QueryRowContext(ctx, query).Scan(&hasPgStatMonitor); err != nil {
		return queryStatsSourceNone, fmt.Errorf("failed to check pg_stat_monitor: %v", err)
	}
	if hasPgStatMonitor {
		c.queryStatsSource = string(queryStatsSourcePgStatMonitor)
		c.pgStatMonitorAvail = true
		return queryStatsSourcePgStatMonitor, nil
	}

	// Fall back to pg_stat_statements
	var hasPgStatStatements bool
	query = `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`
	if err := c.db.QueryRowContext(ctx, query).Scan(&hasPgStatStatements); err != nil {
		return queryStatsSourceNone, fmt.Errorf("failed to check pg_stat_statements: %v", err)
	}
	if hasPgStatStatements {
		c.queryStatsSource = string(queryStatsSourcePgStatStatements)
		c.pgStatStatementsAvail = true
		return queryStatsSourcePgStatStatements, nil
	}

	return queryStatsSourceNone, nil
}
