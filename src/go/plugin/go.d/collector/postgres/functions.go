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
