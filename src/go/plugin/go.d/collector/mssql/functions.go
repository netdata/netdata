// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// mssqlColumnMeta defines metadata for a single column
type mssqlColumnMeta struct {
	dbColumn       string // Column name in sys.query_store_runtime_stats
	alias          string // SQL alias (snake_case for consistency)
	uiKey          string // camelCase key for UI
	displayName    string // Display name in UI
	dataType       string // "string", "integer", "float", "duration"
	units          string // Unit for duration types
	visible        bool   // Default visibility
	transform      string // Transform for value_options
	decimalPoints  int    // Decimal points for display
	sortDir        string // Sort direction: "ascending" or "descending"
	summary        string // Summary function
	filter         string // Filter type
	isMicroseconds bool   // Needs μs to milliseconds conversion
	isSortOption   bool   // Show in sort dropdown
	sortLabel      string // Label for sort option
	isDefaultSort  bool   // Is this the default sort option
	isUniqueKey    bool   // Is this column a unique key
	isSticky       bool   // Is this column sticky in UI
	fullWidth      bool   // Should column take full width
	isIdentity     bool   // Is this an identity column (query_hash, query_text, etc.)
	needsAvg       bool   // Needs weighted average calculation (avg_* columns)
}

// mssqlAllColumns defines ALL possible columns from Query Store
// Columns that don't exist in certain SQL Server versions will be filtered at runtime
var mssqlAllColumns = []mssqlColumnMeta{
	// Identity columns - always available
	{dbColumn: "query_hash", alias: "query_hash", uiKey: "queryHash", displayName: "Query Hash", dataType: "string", visible: false, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isUniqueKey: true, isIdentity: true},
	{dbColumn: "query_sql_text", alias: "query", uiKey: "query", displayName: "Query", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isSticky: true, fullWidth: true, isIdentity: true},
	{dbColumn: "database_name", alias: "database_name", uiKey: "database", displayName: "Database", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isIdentity: true},

	// Execution count - always available
	{dbColumn: "count_executions", alias: "calls", uiKey: "calls", displayName: "Calls", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Number of Calls"},

	// Duration metrics (microseconds -> milliseconds) - SQL 2016+
	{dbColumn: "avg_duration", alias: "total_time", uiKey: "totalTime", displayName: "Total Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range", isMicroseconds: true, isSortOption: true, sortLabel: "Top queries by Total Execution Time", isDefaultSort: true},
	{dbColumn: "avg_duration", alias: "avg_time", uiKey: "avgTime", displayName: "Avg Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "avg", filter: "range", isMicroseconds: true, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Execution Time"},
	{dbColumn: "last_duration", alias: "last_time", uiKey: "lastTime", displayName: "Last Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},
	{dbColumn: "min_duration", alias: "min_time", uiKey: "minTime", displayName: "Min Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "min", filter: "range", isMicroseconds: true},
	{dbColumn: "max_duration", alias: "max_time", uiKey: "maxTime", displayName: "Max Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},
	{dbColumn: "stdev_duration", alias: "stdev_time", uiKey: "stdevTime", displayName: "StdDev Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},

	// CPU time metrics (microseconds -> milliseconds)
	{dbColumn: "avg_cpu_time", alias: "avg_cpu", uiKey: "avgCpu", displayName: "Avg CPU", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "avg", filter: "range", isMicroseconds: true, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average CPU Time"},
	{dbColumn: "last_cpu_time", alias: "last_cpu", uiKey: "lastCpu", displayName: "Last CPU", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},
	{dbColumn: "min_cpu_time", alias: "min_cpu", uiKey: "minCpu", displayName: "Min CPU", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "min", filter: "range", isMicroseconds: true},
	{dbColumn: "max_cpu_time", alias: "max_cpu", uiKey: "maxCpu", displayName: "Max CPU", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},
	{dbColumn: "stdev_cpu_time", alias: "stdev_cpu", uiKey: "stdevCpu", displayName: "StdDev CPU", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},

	// Logical I/O reads
	{dbColumn: "avg_logical_io_reads", alias: "avg_reads", uiKey: "avgReads", displayName: "Avg Logical Reads", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Logical Reads"},
	{dbColumn: "last_logical_io_reads", alias: "last_reads", uiKey: "lastReads", displayName: "Last Logical Reads", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_logical_io_reads", alias: "min_reads", uiKey: "minReads", displayName: "Min Logical Reads", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_logical_io_reads", alias: "max_reads", uiKey: "maxReads", displayName: "Max Logical Reads", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_logical_io_reads", alias: "stdev_reads", uiKey: "stdevReads", displayName: "StdDev Logical Reads", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// Logical I/O writes
	{dbColumn: "avg_logical_io_writes", alias: "avg_writes", uiKey: "avgWrites", displayName: "Avg Logical Writes", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Logical Writes"},
	{dbColumn: "last_logical_io_writes", alias: "last_writes", uiKey: "lastWrites", displayName: "Last Logical Writes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_logical_io_writes", alias: "min_writes", uiKey: "minWrites", displayName: "Min Logical Writes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_logical_io_writes", alias: "max_writes", uiKey: "maxWrites", displayName: "Max Logical Writes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_logical_io_writes", alias: "stdev_writes", uiKey: "stdevWrites", displayName: "StdDev Logical Writes", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// Physical I/O reads
	{dbColumn: "avg_physical_io_reads", alias: "avg_phys_reads", uiKey: "avgPhysReads", displayName: "Avg Physical Reads", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Physical Reads"},
	{dbColumn: "last_physical_io_reads", alias: "last_phys_reads", uiKey: "lastPhysReads", displayName: "Last Physical Reads", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_physical_io_reads", alias: "min_phys_reads", uiKey: "minPhysReads", displayName: "Min Physical Reads", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_physical_io_reads", alias: "max_phys_reads", uiKey: "maxPhysReads", displayName: "Max Physical Reads", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_physical_io_reads", alias: "stdev_phys_reads", uiKey: "stdevPhysReads", displayName: "StdDev Physical Reads", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// CLR time (microseconds -> milliseconds)
	{dbColumn: "avg_clr_time", alias: "avg_clr", uiKey: "avgClr", displayName: "Avg CLR Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "avg", filter: "range", isMicroseconds: true, needsAvg: true},
	{dbColumn: "last_clr_time", alias: "last_clr", uiKey: "lastClr", displayName: "Last CLR Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},
	{dbColumn: "min_clr_time", alias: "min_clr", uiKey: "minClr", displayName: "Min CLR Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "min", filter: "range", isMicroseconds: true},
	{dbColumn: "max_clr_time", alias: "max_clr", uiKey: "maxClr", displayName: "Max CLR Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},
	{dbColumn: "stdev_clr_time", alias: "stdev_clr", uiKey: "stdevClr", displayName: "StdDev CLR Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isMicroseconds: true},

	// DOP (degree of parallelism)
	{dbColumn: "avg_dop", alias: "avg_dop", uiKey: "avgDop", displayName: "Avg DOP", dataType: "float", visible: true, transform: "number", decimalPoints: 1, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Parallelism"},
	{dbColumn: "last_dop", alias: "last_dop", uiKey: "lastDop", displayName: "Last DOP", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_dop", alias: "min_dop", uiKey: "minDop", displayName: "Min DOP", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_dop", alias: "max_dop", uiKey: "maxDop", displayName: "Max DOP", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_dop", alias: "stdev_dop", uiKey: "stdevDop", displayName: "StdDev DOP", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// Memory grant (8KB pages)
	{dbColumn: "avg_query_max_used_memory", alias: "avg_memory", uiKey: "avgMemory", displayName: "Avg Memory (8KB pages)", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Memory Grant"},
	{dbColumn: "last_query_max_used_memory", alias: "last_memory", uiKey: "lastMemory", displayName: "Last Memory (8KB pages)", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_query_max_used_memory", alias: "min_memory", uiKey: "minMemory", displayName: "Min Memory (8KB pages)", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_query_max_used_memory", alias: "max_memory", uiKey: "maxMemory", displayName: "Max Memory (8KB pages)", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_query_max_used_memory", alias: "stdev_memory", uiKey: "stdevMemory", displayName: "StdDev Memory", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// Row count
	{dbColumn: "avg_rowcount", alias: "avg_rows", uiKey: "avgRows", displayName: "Avg Rows", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Row Count"},
	{dbColumn: "last_rowcount", alias: "last_rows", uiKey: "lastRows", displayName: "Last Rows", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_rowcount", alias: "min_rows", uiKey: "minRows", displayName: "Min Rows", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_rowcount", alias: "max_rows", uiKey: "maxRows", displayName: "Max Rows", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_rowcount", alias: "stdev_rows", uiKey: "stdevRows", displayName: "StdDev Rows", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// SQL Server 2017+ log bytes
	{dbColumn: "avg_log_bytes_used", alias: "avg_log_bytes", uiKey: "avgLogBytes", displayName: "Avg Log Bytes", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Log Bytes"},
	{dbColumn: "last_log_bytes_used", alias: "last_log_bytes", uiKey: "lastLogBytes", displayName: "Last Log Bytes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_log_bytes_used", alias: "min_log_bytes", uiKey: "minLogBytes", displayName: "Min Log Bytes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_log_bytes_used", alias: "max_log_bytes", uiKey: "maxLogBytes", displayName: "Max Log Bytes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_log_bytes_used", alias: "stdev_log_bytes", uiKey: "stdevLogBytes", displayName: "StdDev Log Bytes", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},

	// SQL Server 2017+ tempdb space
	{dbColumn: "avg_tempdb_space_used", alias: "avg_tempdb", uiKey: "avgTempdb", displayName: "Avg TempDB (8KB pages)", dataType: "float", visible: true, transform: "number", decimalPoints: 0, sortDir: "descending", summary: "avg", filter: "range", needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average TempDB Usage"},
	{dbColumn: "last_tempdb_space_used", alias: "last_tempdb", uiKey: "lastTempdb", displayName: "Last TempDB (8KB pages)", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_tempdb_space_used", alias: "min_tempdb", uiKey: "minTempdb", displayName: "Min TempDB (8KB pages)", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_tempdb_space_used", alias: "max_tempdb", uiKey: "maxTempdb", displayName: "Max TempDB (8KB pages)", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stdev_tempdb_space_used", alias: "stdev_tempdb", uiKey: "stdevTempdb", displayName: "StdDev TempDB", dataType: "float", visible: false, transform: "number", sortDir: "descending", summary: "max", filter: "range"},
}

// mssqlMethods returns the available function methods for MSSQL
func mssqlMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []module.SortOption
	seen := make(map[string]bool) // Avoid duplicates from totalTime/avgTime using same dbColumn
	for _, col := range mssqlAllColumns {
		if col.isSortOption && !seen[col.uiKey] {
			seen[col.uiKey] = true
			sortOptions = append(sortOptions, module.SortOption{
				ID:      col.uiKey,
				Column:  col.uiKey, // Use UI key for sort, we'll map internally
				Label:   col.sortLabel,
				Default: col.isDefaultSort,
			})
		}
	}

	return []module.MethodConfig{{
		ID:          "top-queries",
		Name:        "Top Queries",
		Help:        "Top SQL queries from Query Store",
		SortOptions: sortOptions,
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

// detectMSSQLQueryStoreColumns queries the database to discover available columns
func (c *Collector) detectMSSQLQueryStoreColumns(ctx context.Context) (map[string]bool, error) {
	// Fast path: return cached result
	c.queryStoreColsMu.RLock()
	if c.queryStoreCols != nil {
		cols := c.queryStoreCols
		c.queryStoreColsMu.RUnlock()
		return cols, nil
	}
	c.queryStoreColsMu.RUnlock()

	// Slow path: query and cache
	c.queryStoreColsMu.Lock()
	defer c.queryStoreColsMu.Unlock()

	// Double-check after acquiring write lock
	if c.queryStoreCols != nil {
		return c.queryStoreCols, nil
	}

	// Query sys.columns to get available columns
	query := `
		SELECT name
		FROM sys.columns
		WHERE object_id = OBJECT_ID('sys.query_store_runtime_stats')
	`
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query column information: %w", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, fmt.Errorf("failed to scan column name: %w", err)
		}
		cols[colName] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating columns: %w", err)
	}

	// Add identity columns that are always available
	cols["query_hash"] = true
	cols["query_sql_text"] = true
	cols["database_name"] = true

	// Cache the result
	c.queryStoreCols = cols

	return cols, nil
}

// buildAvailableMSSQLColumns filters columns based on what's available in the database
func (c *Collector) buildAvailableMSSQLColumns(availableCols map[string]bool) []mssqlColumnMeta {
	var cols []mssqlColumnMeta
	seen := make(map[string]bool)

	for _, col := range mssqlAllColumns {
		// Skip duplicates (e.g., totalTime and avgTime both use avg_duration)
		if seen[col.uiKey] {
			continue
		}
		// Identity columns are always available
		if col.isIdentity {
			cols = append(cols, col)
			seen[col.uiKey] = true
			continue
		}
		// Check if the dbColumn exists
		if availableCols[col.dbColumn] {
			cols = append(cols, col)
			seen[col.uiKey] = true
		}
	}
	return cols
}

// mapAndValidateMSSQLSortColumn maps UI sort key to the appropriate sort expression
func (c *Collector) mapAndValidateMSSQLSortColumn(sortKey string, availableCols map[string]bool) string {
	// Find the column mapping
	for _, col := range mssqlAllColumns {
		if col.uiKey == sortKey {
			// Check if the underlying column is available
			if col.isIdentity || availableCols[col.dbColumn] {
				return col.uiKey // Return UI key, we'll build the right expression in SQL
			}
		}
	}
	// Default to totalTime
	return "totalTime"
}

// buildMSSQLDynamicSQL builds the SQL query with only available columns
func (c *Collector) buildMSSQLDynamicSQL(cols []mssqlColumnMeta, sortColumn string, timeWindowDays int, limit int) string {
	var selectParts []string

	for _, col := range cols {
		var expr string
		switch {
		case col.isIdentity:
			switch col.alias {
			case "query_hash":
				expr = fmt.Sprintf("CONVERT(VARCHAR(64), q.query_hash, 1) AS %s", col.alias)
			case "query":
				expr = fmt.Sprintf("qt.query_sql_text AS %s", col.alias)
			case "database_name":
				expr = fmt.Sprintf("DB_NAME() AS %s", col.alias)
			}
		case col.alias == "calls":
			expr = fmt.Sprintf("SUM(rs.count_executions) AS %s", col.alias)
		case col.alias == "total_time":
			// Total time = sum of (avg_duration * executions) converted to milliseconds
			expr = fmt.Sprintf("SUM(rs.avg_duration * rs.count_executions) / 1000.0 AS %s", col.alias)
		case col.needsAvg && col.isMicroseconds:
			// Weighted average with μs to milliseconds conversion
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) / 1000.0 ELSE 0 END AS %s", col.dbColumn, col.alias)
		case col.needsAvg:
			// Weighted average without time conversion
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) ELSE 0 END AS %s", col.dbColumn, col.alias)
		case col.isMicroseconds:
			// Aggregate with μs to milliseconds conversion
			aggFunc := "MAX"
			if strings.HasPrefix(col.dbColumn, "min_") {
				aggFunc = "MIN"
			}
			expr = fmt.Sprintf("%s(rs.%s) / 1000.0 AS %s", aggFunc, col.dbColumn, col.alias)
		default:
			// Simple aggregate
			aggFunc := "MAX"
			if strings.HasPrefix(col.dbColumn, "min_") {
				aggFunc = "MIN"
			}
			if strings.HasPrefix(col.dbColumn, "stdev_") {
				aggFunc = "MAX" // Use MAX for stddev aggregation
			}
			expr = fmt.Sprintf("%s(rs.%s) AS %s", aggFunc, col.dbColumn, col.alias)
		}
		if expr != "" {
			selectParts = append(selectParts, expr)
		}
	}

	// Time window filter
	timeFilter := ""
	if timeWindowDays > 0 {
		timeFilter = fmt.Sprintf("WHERE rsi.start_time >= DATEADD(day, -%d, GETUTCDATE())", timeWindowDays)
	}

	// Map UI sort key to SQL alias - look up from column metadata
	orderByExpr := "total_time" // default
	for _, col := range cols {
		if col.uiKey == sortColumn {
			orderByExpr = col.alias
			break
		}
	}

	return fmt.Sprintf(`
SELECT TOP %d
    %s
FROM sys.query_store_query q
JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
JOIN sys.query_store_plan p ON q.query_id = p.query_id
JOIN sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
JOIN sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
%s
GROUP BY q.query_hash, qt.query_sql_text
ORDER BY %s DESC
`, limit, strings.Join(selectParts, ",\n    "), timeFilter, orderByExpr)
}

// mssqlRowScanner interface for testing
type mssqlRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// scanMSSQLDynamicRows scans rows dynamically based on column types
func (c *Collector) scanMSSQLDynamicRows(rows mssqlRowScanner, cols []mssqlColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))

		for i, col := range cols {
			switch col.dataType {
			case "string":
				var v sql.NullString
				values[i] = &v
			case "integer":
				var v sql.NullInt64
				values[i] = &v
			case "float", "duration":
				var v sql.NullFloat64
				values[i] = &v
			default:
				var v any
				values[i] = &v
			}
			valuePtrs[i] = values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}

		// Convert scanned values to output format
		row := make([]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case *sql.NullString:
				if v.Valid {
					s := v.String
					// Truncate query text
					if col.uiKey == "query" {
						s = strmutil.TruncateText(s, maxQueryTextLength)
					}
					row[i] = s
				} else {
					row[i] = ""
				}
			case *sql.NullInt64:
				if v.Valid {
					row[i] = v.Int64
				} else {
					row[i] = int64(0)
				}
			case *sql.NullFloat64:
				if v.Valid {
					row[i] = v.Float64
				} else {
					row[i] = float64(0)
				}
			default:
				row[i] = nil
			}
		}
		data = append(data, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return data, nil
}

// buildMSSQLDynamicColumns builds column definitions for the response
func (c *Collector) buildMSSQLDynamicColumns(cols []mssqlColumnMeta) map[string]any {
	columns := make(map[string]any)
	for i, col := range cols {
		colDef := map[string]any{
			"index":                   i,
			"unique_key":              col.isUniqueKey,
			"name":                    col.displayName,
			"type":                    col.dataType,
			"visible":                 col.visible,
			"visualization":           "value",
			"sort":                    col.sortDir,
			"sortable":                true,
			"sticky":                  col.isSticky,
			"summary":                 col.summary,
			"filter":                  col.filter,
			"full_width":              col.fullWidth,
			"wrap":                    false,
			"default_expanded_filter": false,
		}

		// Add value_options
		valueOpts := map[string]any{
			"transform":      col.transform,
			"decimal_points": col.decimalPoints,
			"default_value":  nil,
		}
		if col.units != "" {
			valueOpts["units"] = col.units
			colDef["units"] = col.units
		}
		colDef["value_options"] = valueOpts

		// Duration columns use bar visualization
		if col.dataType == "duration" {
			colDef["visualization"] = "bar"
		}

		columns[col.uiKey] = colDef
	}
	return columns
}

// collectTopQueries queries Query Store for top queries using dynamic columns
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Check if function is enabled
	if !c.Config.GetQueryStoreFunctionEnabled() {
		return &module.FunctionResponse{
			Status: 403,
			Message: "Query Store function has been disabled in configuration. " +
				"To enable, set query_store_function_enabled: true in the MSSQL collector config.",
		}
	}

	// Detect available columns
	availableCols, err := c.detectMSSQLQueryStoreColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	// Build list of available columns
	cols := c.buildAvailableMSSQLColumns(availableCols)
	if len(cols) == 0 {
		return &module.FunctionResponse{
			Status:  500,
			Message: "no columns available in Query Store",
		}
	}

	// Validate and map sort column
	validatedSortColumn := c.mapAndValidateMSSQLSortColumn(sortColumn, availableCols)

	// Build and execute query
	timeWindowDays := c.Config.GetQueryStoreTimeWindowDays()
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}
	query := c.buildMSSQLDynamicSQL(cols, validatedSortColumn, timeWindowDays, limit)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	// Scan rows dynamically
	data, err := c.scanMSSQLDynamicRows(rows, cols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	// Find default sort column UI key
	defaultSort := "totalTime"
	for _, col := range cols {
		if col.isDefaultSort {
			defaultSort = col.uiKey
			break
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           c.buildMSSQLDynamicColumns(cols),
		Data:              data,
		DefaultSortColumn: defaultSort,

		// Charts for aggregated visualization
		Charts: map[string]module.ChartConfig{
			"Calls": {
				Name:    "Number of Calls",
				Type:    "stacked-bar",
				Columns: []string{"calls"},
			},
			"Time": {
				Name:    "Execution Time",
				Type:    "stacked-bar",
				Columns: []string{"totalTime", "avgTime"},
			},
			"CPU": {
				Name:    "CPU Time",
				Type:    "stacked-bar",
				Columns: []string{"avgCpu"},
			},
			"IO": {
				Name:    "Logical I/O",
				Type:    "stacked-bar",
				Columns: []string{"avgReads", "avgWrites"},
			},
		},
		DefaultCharts: [][]string{
			{"Time", "database"},
			{"Calls", "database"},
		},
		GroupBy: map[string]module.GroupByConfig{
			"database": {
				Name:    "Group by Database",
				Columns: []string{"database"},
			},
		},
	}
}
