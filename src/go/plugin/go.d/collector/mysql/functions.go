// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// mysqlValidSortColumns defines the allowed sort columns for defense-in-depth
var mysqlValidSortColumns = map[string]bool{
	"SUM_TIMER_WAIT":    true,
	"COUNT_STAR":        true,
	"AVG_TIMER_WAIT":    true,
	"SUM_ROWS_SENT":     true,
	"SUM_ROWS_EXAMINED": true,
	"SUM_NO_INDEX_USED": true,
}

// mysqlMethods returns the available function methods for MySQL
func mysqlMethods() []module.MethodConfig {
	return []module.MethodConfig{{
		ID:   "top-queries",
		Name: "Top Queries",
		Help: "Top SQL queries from performance_schema",
		SortOptions: []module.SortOption{
			{ID: "total_time", Column: "SUM_TIMER_WAIT", Label: "Top 5k queries by Total Execution Time", Default: true},
			{ID: "calls", Column: "COUNT_STAR", Label: "Top 5k queries by Number of Calls"},
			{ID: "avg_time", Column: "AVG_TIMER_WAIT", Label: "Top 5k queries by Average Execution Time"},
			{ID: "rows_sent", Column: "SUM_ROWS_SENT", Label: "Top 5k queries by Rows Sent"},
			{ID: "rows_examined", Column: "SUM_ROWS_EXAMINED", Label: "Top 5k queries by Rows Examined"},
			{ID: "no_index", Column: "SUM_NO_INDEX_USED", Label: "Top 5k queries by No Index Used"},
		},
	}}
}

// mysqlHandleMethod handles function requests for MySQL
func mysqlHandleMethod(ctx context.Context, job *module.Job, method, sortColumn string) *module.FunctionResponse {
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

// collectTopQueries queries performance_schema.events_statements_summary_by_digest for top queries
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Check if performance_schema is enabled (query directly to avoid race condition)
	available, err := c.checkPerformanceSchema(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to check performance_schema availability: %v", err),
		}
	}
	if !available {
		return &module.FunctionResponse{
			Status:  503,
			Message: "performance_schema is not enabled",
		}
	}

	// Defense-in-depth: validate sort column against whitelist
	if !mysqlValidSortColumns[sortColumn] {
		sortColumn = "SUM_TIMER_WAIT" // safe default
	}

	// Build and execute query
	// MySQL timers are in PICOSECONDS (10^-12 seconds)
	// To convert to MILLISECONDS (10^-3 seconds): divide by 10^9
	query := fmt.Sprintf(`
SELECT
    DIGEST,
    DIGEST_TEXT,
    IFNULL(SCHEMA_NAME, '') AS schema_name,
    COUNT_STAR AS calls,
    SUM_TIMER_WAIT/1000000000 AS total_time_ms,
    AVG_TIMER_WAIT/1000000000 AS avg_time_ms,
    MIN_TIMER_WAIT/1000000000 AS min_time_ms,
    MAX_TIMER_WAIT/1000000000 AS max_time_ms,
    SUM_ROWS_SENT,
    SUM_ROWS_EXAMINED,
    SUM_NO_INDEX_USED,
    SUM_NO_GOOD_INDEX_USED
FROM performance_schema.events_statements_summary_by_digest
WHERE DIGEST IS NOT NULL
ORDER BY %s DESC
LIMIT 5000
`, sortColumn)

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
		var digest, digestText, schemaName string
		var calls int64
		var totalTime, avgTime, minTime, maxTime float64
		var rowsSent, rowsExamined, noIndexUsed, noGoodIndexUsed int64

		if err := rows.Scan(
			&digest, &digestText, &schemaName,
			&calls,
			&totalTime, &avgTime, &minTime, &maxTime,
			&rowsSent, &rowsExamined, &noIndexUsed, &noGoodIndexUsed,
		); err != nil {
			return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("row scan failed: %v", err)}
		}

		// Truncate query text to prevent excessive memory usage
		digestText = strmutil.TruncateText(digestText, maxQueryTextLength)

		// Duration values are converted from ms to seconds (UI expects seconds)
		data = append(data, map[string]any{
			"digest":             digest,
			"query":              digestText,
			"schema":             schemaName,
			"calls":              calls,
			"total_time":         totalTime / 1000.0,
			"avg_time":           avgTime / 1000.0,
			"min_time":           minTime / 1000.0,
			"max_time":           maxTime / 1000.0,
			"rows_sent":          rowsSent,
			"rows_examined":      rowsExamined,
			"no_index_used":      noIndexUsed,
			"no_good_index_used": noGoodIndexUsed,
		})
	}

	if err := rows.Err(); err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("rows iteration error: %v", err)}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from performance_schema.events_statements_summary_by_digest",
		Columns:           buildMySQLTopQueriesColumns(),
		Data:              data,
		DefaultSortColumn: "total_time",
	}
}

// buildMySQLTopQueriesColumns builds column definitions for the response
// Column schema follows Netdata Functions v3 format with index, unique_key, visible fields
func buildMySQLTopQueriesColumns() map[string]any {
	return map[string]any{
		"digest": map[string]any{
			"index":      0,
			"unique_key": true,
			"name":       "Digest",
			"type":       "string",
			"visible":    false,
		},
		"query": map[string]any{
			"index":      1,
			"name":       "Query",
			"type":       "string",
			"visible":    true,
			"full_width": true,
		},
		"schema": map[string]any{
			"index":   2,
			"name":    "Schema",
			"type":    "string",
			"visible": true,
		},
		"calls": map[string]any{
			"index":   3,
			"name":    "Calls",
			"type":    "integer",
			"visible": true,
		},
		"total_time": map[string]any{
			"index":   4,
			"name":    "Total Time",
			"type":    "duration",
			"visible": true,
		},
		"avg_time": map[string]any{
			"index":   5,
			"name":    "Avg Time",
			"type":    "duration",
			"visible": true,
		},
		"min_time": map[string]any{
			"index":   6,
			"name":    "Min Time",
			"type":    "duration",
			"visible": false,
		},
		"max_time": map[string]any{
			"index":   7,
			"name":    "Max Time",
			"type":    "duration",
			"visible": false,
		},
		"rows_sent": map[string]any{
			"index":   8,
			"name":    "Rows Sent",
			"type":    "integer",
			"visible": true,
		},
		"rows_examined": map[string]any{
			"index":   9,
			"name":    "Rows Examined",
			"type":    "integer",
			"visible": true,
		},
		"no_index_used": map[string]any{
			"index":   10,
			"name":    "No Index Used",
			"type":    "integer",
			"visible": false,
		},
		"no_good_index_used": map[string]any{
			"index":   11,
			"name":    "No Good Index Used",
			"type":    "integer",
			"visible": false,
		},
	}
}

// checkPerformanceSchema checks if performance_schema is enabled (cached)
func (c *Collector) checkPerformanceSchema(ctx context.Context) (bool, error) {
	// Fast path: return cached result if already checked
	c.varPerfSchemaMu.RLock()
	cached := c.varPerformanceSchema
	c.varPerfSchemaMu.RUnlock()
	if cached != "" {
		return cached == "ON" || cached == "1", nil
	}

	// Slow path: query and cache the result
	// Use write lock for the entire operation to prevent duplicate queries
	c.varPerfSchemaMu.Lock()
	defer c.varPerfSchemaMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have set it)
	if c.varPerformanceSchema != "" {
		return c.varPerformanceSchema == "ON" || c.varPerformanceSchema == "1", nil
	}

	var value string
	query := "SELECT @@performance_schema"
	err := c.db.QueryRowContext(ctx, query).Scan(&value)
	if err != nil {
		return false, err
	}

	// Cache the result
	c.varPerformanceSchema = value
	return value == "ON" || value == "1", nil
}
