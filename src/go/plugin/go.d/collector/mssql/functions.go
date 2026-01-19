// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// mssqlValidSortColumns defines the allowed sort columns for defense-in-depth
var mssqlValidSortColumns = map[string]bool{
	"total_time_ms": true,
	"calls":         true,
	"avg_time_ms":   true,
	"avg_cpu_ms":    true,
	"avg_reads":     true,
	"avg_writes":    true,
}

// mssqlMethods returns the available function methods for MSSQL
func mssqlMethods() []module.MethodConfig {
	return []module.MethodConfig{{
		ID:   "top-queries",
		Name: "Top Queries",
		Help: "Top SQL queries from Query Store",
		SortOptions: []module.SortOption{
			{ID: "total_time", Column: "total_time_ms", Label: "Top 5k queries by Total Execution Time", Default: true},
			{ID: "calls", Column: "calls", Label: "Top 5k queries by Number of Calls"},
			{ID: "avg_time", Column: "avg_time_ms", Label: "Top 5k queries by Average Execution Time"},
			{ID: "avg_cpu", Column: "avg_cpu_ms", Label: "Top 5k queries by Average CPU Time"},
			{ID: "avg_reads", Column: "avg_reads", Label: "Top 5k queries by Average Logical Reads"},
			{ID: "avg_writes", Column: "avg_writes", Label: "Top 5k queries by Average Logical Writes"},
		},
	}}
}

// mssqlHandleMethod handles function requests for MSSQL
func mssqlHandleMethod(ctx context.Context, job *module.Job, method, sortColumn string) *module.FunctionResponse {
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

// collectTopQueries queries Query Store for top queries
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Check if function is enabled (opt-in due to PII risk)
	if !c.Config.GetQueryStoreFunctionEnabled() {
		return &module.FunctionResponse{
			Status: 403,
			Message: "Query Store function is disabled by default due to PII risk. " +
				"MSSQL Query Store may contain unmasked literal values (customer names, emails, IDs). " +
				"To enable, set query_store_function_enabled: true in the MSSQL collector config " +
				"after ensuring proper access controls to the Netdata dashboard.",
		}
	}

	// Defense-in-depth: validate sort column against whitelist
	if !mssqlValidSortColumns[sortColumn] {
		sortColumn = "total_time_ms" // safe default
	}

	// Build and execute query
	timeWindowDays := c.Config.GetQueryStoreTimeWindowDays()
	query := c.buildQueryStoreSQL(sortColumn, timeWindowDays)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		// Check if Query Store is not available
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	// Process rows and build response
	data := make([]map[string]any, 0, 5000)
	for rows.Next() {
		var queryHash string
		var queryText string
		var dbName string
		var calls int64
		var totalTime, avgTime, avgCPU, avgReads, avgWrites float64

		if err := rows.Scan(
			&queryHash, &queryText, &dbName,
			&calls, &totalTime, &avgTime, &avgCPU, &avgReads, &avgWrites,
		); err != nil {
			return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("row scan failed: %v", err)}
		}

		// Truncate query text to prevent excessive memory usage
		queryText = strmutil.TruncateText(queryText, maxQueryTextLength)

		// Duration values are converted from ms to seconds (UI expects seconds)
		data = append(data, map[string]any{
			"query_hash": queryHash,
			"query":      queryText,
			"database":   dbName,
			"calls":      calls,
			"total_time": totalTime / 1000.0,
			"avg_time":   avgTime / 1000.0,
			"avg_cpu":    avgCPU / 1000.0,
			"avg_reads":  avgReads,
			"avg_writes": avgWrites,
		})
	}

	if err := rows.Err(); err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("rows iteration error: %v", err)}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildMSSQLTopQueriesColumns(),
		Data:              data,
		DefaultSortColumn: "total_time",
	}
}

// buildQueryStoreSQL builds the SQL query for Query Store
func (c *Collector) buildQueryStoreSQL(sortColumn string, timeWindowDays int) string {
	// Time window filter
	timeFilter := ""
	if timeWindowDays > 0 {
		timeFilter = fmt.Sprintf("WHERE rsi.start_time >= DATEADD(day, -%d, GETUTCDATE())", timeWindowDays)
	}

	// MSSQL stores duration in MICROSECONDS, divide by 1000 for ms
	return fmt.Sprintf(`
SELECT TOP 5000
    CONVERT(VARCHAR(64), q.query_hash, 1) AS query_hash,
    qt.query_sql_text,
    DB_NAME() AS database_name,
    SUM(rs.count_executions) AS calls,
    SUM(rs.avg_duration * rs.count_executions) / 1000.0 AS total_time_ms,
    CASE WHEN SUM(rs.count_executions) > 0
         THEN SUM(rs.avg_duration * rs.count_executions) / SUM(rs.count_executions) / 1000.0
         ELSE 0 END AS avg_time_ms,
    CASE WHEN SUM(rs.count_executions) > 0
         THEN SUM(rs.avg_cpu_time * rs.count_executions) / SUM(rs.count_executions) / 1000.0
         ELSE 0 END AS avg_cpu_ms,
    CASE WHEN SUM(rs.count_executions) > 0
         THEN SUM(rs.avg_logical_io_reads * rs.count_executions) / SUM(rs.count_executions)
         ELSE 0 END AS avg_reads,
    CASE WHEN SUM(rs.count_executions) > 0
         THEN SUM(rs.avg_logical_io_writes * rs.count_executions) / SUM(rs.count_executions)
         ELSE 0 END AS avg_writes
FROM sys.query_store_query q
JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
JOIN sys.query_store_plan p ON q.query_id = p.query_id
JOIN sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
JOIN sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
%s
GROUP BY q.query_hash, qt.query_sql_text
ORDER BY %s DESC
`, timeFilter, sortColumn)
}

// buildMSSQLTopQueriesColumns builds column definitions for the response
// Column schema follows Netdata Functions v3 format with index, unique_key, visible fields
func buildMSSQLTopQueriesColumns() map[string]any {
	return map[string]any{
		"query_hash": map[string]any{
			"index":      0,
			"unique_key": true,
			"name":       "Query Hash",
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
		"database": map[string]any{
			"index":   2,
			"name":    "Database",
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
		"avg_cpu": map[string]any{
			"index":   6,
			"name":    "Avg CPU",
			"type":    "duration",
			"visible": true,
		},
		"avg_reads": map[string]any{
			"index":   7,
			"name":    "Avg Logical Reads",
			"type":    "number",
			"visible": true,
		},
		"avg_writes": map[string]any{
			"index":   8,
			"name":    "Avg Logical Writes",
			"type":    "number",
			"visible": false,
		},
	}
}
