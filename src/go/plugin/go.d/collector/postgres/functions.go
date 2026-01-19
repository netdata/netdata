// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// pgMethods returns the available function methods for PostgreSQL
func pgMethods() []module.MethodConfig {
	return []module.MethodConfig{{
		ID:   "top-queries",
		Name: "Top Queries",
		Help: "Top SQL queries from pg_stat_statements",
		SortOptions: []module.SortOption{
			{ID: "total_time", Column: "total_time", Label: "Top 5k queries by Total Execution Time", Default: true},
			{ID: "calls", Column: "calls", Label: "Top 5k queries by Number of Calls"},
			{ID: "mean_time", Column: "mean_time", Label: "Top 5k queries by Average Execution Time"},
			{ID: "rows", Column: "rows", Label: "Top 5k queries by Rows Returned"},
			{ID: "shared_blks_read", Column: "shared_blks_read", Label: "Top 5k queries by Disk Reads (I/O)"},
			{ID: "temp_blks_written", Column: "temp_blks_written", Label: "Top 5k queries by Temp Writes"},
		},
	}}
}

// pgHandleMethod handles function requests for PostgreSQL
func pgHandleMethod(ctx context.Context, job *module.Job, method, sortColumn string) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	// Check if collector is initialized (first collect() may not have run yet)
	if collector.db == nil {
		return &module.FunctionResponse{
			Status:  503,
			Message: "collector is still initializing, please retry in a few seconds",
		}
	}

	switch method {
	case "top-queries":
		return collector.collectTopQueries(ctx, sortColumn)
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

// collectTopQueries queries pg_stat_statements for top queries
func (c *Collector) collectTopQueries(ctx context.Context, semanticSortCol string) *module.FunctionResponse {
	// Check pg_stat_statements availability (lazy check)
	available, err := c.checkPgStatStatements(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to check pg_stat_statements availability: %v", err),
		}
	}
	if !available {
		return &module.FunctionResponse{
			Status:  503,
			Message: "pg_stat_statements extension is not installed or not available",
		}
	}

	// Map semantic column to actual SQL column for this PG version
	actualSortCol := c.mapSortColumn(semanticSortCol)

	// Build and execute query
	query := c.buildTopQueriesSQL(actualSortCol)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	// Process rows and build response
	data := make([]map[string]any, 0, 5000)
	for rows.Next() {
		var queryID int64
		var queryText, dbName, userName string
		var calls, totalRows int64
		var totalTime, meanTime, minTime, maxTime float64
		var sharedBlksHit, sharedBlksRead, tempBlksWritten int64

		if err := rows.Scan(
			&queryID, &queryText, &dbName, &userName,
			&calls, &totalTime, &meanTime, &minTime, &maxTime,
			&totalRows, &sharedBlksHit, &sharedBlksRead, &tempBlksWritten,
		); err != nil {
			return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("row scan failed: %v", err)}
		}

		// Truncate query text to prevent excessive memory usage
		queryText = strmutil.TruncateText(queryText, maxQueryTextLength)

		// Note: queryID is converted to string to avoid JavaScript integer precision loss
		// (JS safe integer max is 2^53-1, pg queryid can exceed this)
		// Duration values are converted from ms to seconds (UI expects seconds)
		data = append(data, map[string]any{
			"queryid":           fmt.Sprintf("%d", queryID),
			"query":             queryText,
			"database":          dbName,
			"user":              userName,
			"calls":             calls,
			"total_time":        totalTime / 1000.0,
			"mean_time":         meanTime / 1000.0,
			"min_time":          minTime / 1000.0,
			"max_time":          maxTime / 1000.0,
			"rows":              totalRows,
			"shared_blks_hit":   sharedBlksHit,
			"shared_blks_read":  sharedBlksRead,
			"temp_blks_written": tempBlksWritten,
		})
	}

	if err := rows.Err(); err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("rows iteration error: %v", err)}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements",
		Columns:           c.buildTopQueriesColumns(),
		Data:              data,
		DefaultSortColumn: "total_time",
	}
}

// checkPgStatStatements checks if pg_stat_statements extension is available (cached)
func (c *Collector) checkPgStatStatements(ctx context.Context) (bool, error) {
	// Fast path: return cached result if already checked
	c.pgStatStatementsMu.RLock()
	checked := c.pgStatStatementsChecked
	avail := c.pgStatStatementsAvail
	c.pgStatStatementsMu.RUnlock()
	if checked {
		return avail, nil
	}

	// Slow path: query and cache the result
	// Use write lock for the entire operation to prevent duplicate queries
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have set it)
	if c.pgStatStatementsChecked {
		return c.pgStatStatementsAvail, nil
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`
	err := c.db.QueryRowContext(ctx, query).Scan(&exists)
	if err != nil {
		return false, err
	}

	// Cache the result
	c.pgStatStatementsAvail = exists
	c.pgStatStatementsChecked = true
	return exists, nil
}

// pgValidSortColumns defines the allowed sort columns for defense-in-depth
var pgValidSortColumns = map[string]bool{
	"total_time":        true,
	"calls":             true,
	"mean_time":         true,
	"rows":              true,
	"shared_blks_read":  true,
	"temp_blks_written": true,
	// PG 13+ column names (mapped internally)
	"total_exec_time": true,
	"mean_exec_time":  true,
	"min_exec_time":   true,
	"max_exec_time":   true,
}

// mapSortColumn maps semantic column IDs to actual SQL columns based on PG version
// Defense-in-depth: validates column against whitelist before returning
func (c *Collector) mapSortColumn(semanticCol string) string {
	var result string

	if c.pgVersion >= pgVersion13 {
		// PG 13+ uses _exec_ suffix for time columns
		switch semanticCol {
		case "total_time":
			result = "total_exec_time"
		case "mean_time":
			result = "mean_exec_time"
		case "min_time":
			result = "min_exec_time"
		case "max_time":
			result = "max_exec_time"
		default:
			result = semanticCol
		}
	} else {
		result = semanticCol
	}

	// Defense-in-depth: validate against whitelist
	if !pgValidSortColumns[result] {
		// Fall back to safe default if unrecognized
		if c.pgVersion >= pgVersion13 {
			return "total_exec_time"
		}
		return "total_time"
	}

	return result
}

// buildTopQueriesSQL builds the SQL query for top queries
func (c *Collector) buildTopQueriesSQL(sortColumn string) string {
	// Select version-appropriate column names
	timeColumns := "total_time, mean_time, min_time, max_time"
	if c.pgVersion >= pgVersion13 {
		timeColumns = "total_exec_time AS total_time, mean_exec_time AS mean_time, min_exec_time AS min_time, max_exec_time AS max_time"
	}

	return fmt.Sprintf(`
SELECT
    s.queryid,
    s.query,
    d.datname AS database,
    u.usename AS user,
    s.calls,
    %s,
    s.rows,
    s.shared_blks_hit,
    s.shared_blks_read,
    s.temp_blks_written
FROM pg_stat_statements s
JOIN pg_database d ON s.dbid = d.oid
JOIN pg_user u ON s.userid = u.usesysid
ORDER BY %s DESC
LIMIT 5000
`, timeColumns, sortColumn)
}

// buildTopQueriesColumns builds column definitions for the response
// Column schema follows Netdata Functions v3 format with index, unique_key, visible fields
func (c *Collector) buildTopQueriesColumns() map[string]any {
	return map[string]any{
		"queryid": map[string]any{
			"index":      0,
			"unique_key": true,
			"name":       "Query ID",
			"type":       "string", // string to avoid JS integer precision loss
			"visible":    false,
		},
		"query": map[string]any{
			"index":      1,
			"name":       "Query",
			"type":       "string",
			"visible":    true,
			"full_width": true,
		},
		"database": map[string]any{
			"index":   2,
			"name":    "Database",
			"type":    "string",
			"visible": true,
		},
		"user": map[string]any{
			"index":   3,
			"name":    "User",
			"type":    "string",
			"visible": true,
		},
		"calls": map[string]any{
			"index":   4,
			"name":    "Calls",
			"type":    "integer",
			"visible": true,
		},
		"total_time": map[string]any{
			"index":   5,
			"name":    "Total Time",
			"type":    "duration",
			"visible": true,
		},
		"mean_time": map[string]any{
			"index":   6,
			"name":    "Mean Time",
			"type":    "duration",
			"visible": true,
		},
		"min_time": map[string]any{
			"index":   7,
			"name":    "Min Time",
			"type":    "duration",
			"visible": false,
		},
		"max_time": map[string]any{
			"index":   8,
			"name":    "Max Time",
			"type":    "duration",
			"visible": false,
		},
		"rows": map[string]any{
			"index":   9,
			"name":    "Rows",
			"type":    "integer",
			"visible": true,
		},
		"shared_blks_hit": map[string]any{
			"index":   10,
			"name":    "Shared Blocks Hit",
			"type":    "integer",
			"visible": false,
		},
		"shared_blks_read": map[string]any{
			"index":   11,
			"name":    "Shared Blocks Read",
			"type":    "integer",
			"visible": true,
		},
		"temp_blks_written": map[string]any{
			"index":   12,
			"name":    "Temp Blocks Written",
			"type":    "integer",
			"visible": false,
		},
	}
}
