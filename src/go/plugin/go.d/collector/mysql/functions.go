// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// mysqlColumnMeta defines metadata for a single column
type mysqlColumnMeta struct {
	dbColumn      string // Column name in database (e.g., "SUM_TIMER_WAIT")
	alias         string // SQL alias (snake_case for consistency)
	uiKey         string // camelCase key for UI (e.g., "totalTime")
	displayName   string // Display name in UI (e.g., "Total Time")
	dataType      string // "string", "integer", "float", "duration"
	units         string // Unit for duration/numeric types (e.g., "seconds")
	visible       bool   // Default visibility
	transform     string // Transform for value_options (e.g., "duration", "number", "none")
	decimalPoints int    // Decimal points for display
	sortDir       string // Sort direction: "ascending" or "descending"
	summary       string // Summary function: "sum", "count", "max", "min", "avg"
	filter        string // Filter type: "multiselect" or "range"
	isPicoseconds bool   // Needs picoseconds to seconds conversion
	isSortOption  bool   // Show in sort dropdown
	sortLabel     string // Label for sort option
	isDefaultSort bool   // Is this the default sort option
	isUniqueKey   bool   // Is this column a unique key
	isSticky      bool   // Is this column sticky in UI
	fullWidth     bool   // Should column take full width
}

// mysqlAllColumns defines ALL possible columns from events_statements_summary_by_digest
// Columns that don't exist in certain MySQL/MariaDB versions will be filtered at runtime
var mysqlAllColumns = []mysqlColumnMeta{
	// Identity columns - always available
	{dbColumn: "DIGEST", alias: "digest", uiKey: "digest", displayName: "Digest", dataType: "string", visible: false, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isUniqueKey: true},
	{dbColumn: "DIGEST_TEXT", alias: "query", uiKey: "query", displayName: "Query", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isSticky: true, fullWidth: true},
	{dbColumn: "SCHEMA_NAME", alias: "schema_name", uiKey: "schema", displayName: "Schema", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},

	// Execution counts
	{dbColumn: "COUNT_STAR", alias: "calls", uiKey: "calls", displayName: "Calls", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Number of Calls"},

	// Timer metrics (picoseconds -> seconds)
	{dbColumn: "SUM_TIMER_WAIT", alias: "total_time", uiKey: "totalTime", displayName: "Total Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range", isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by Total Execution Time", isDefaultSort: true},
	{dbColumn: "MIN_TIMER_WAIT", alias: "min_time", uiKey: "minTime", displayName: "Min Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "min", filter: "range", isPicoseconds: true},
	{dbColumn: "AVG_TIMER_WAIT", alias: "avg_time", uiKey: "avgTime", displayName: "Avg Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "avg", filter: "range", isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by Average Execution Time"},
	{dbColumn: "MAX_TIMER_WAIT", alias: "max_time", uiKey: "maxTime", displayName: "Max Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isPicoseconds: true},

	// Lock time (picoseconds -> seconds)
	{dbColumn: "SUM_LOCK_TIME", alias: "lock_time", uiKey: "lockTime", displayName: "Lock Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range", isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by Lock Time"},

	// Error and warning counts
	{dbColumn: "SUM_ERRORS", alias: "errors", uiKey: "errors", displayName: "Errors", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Errors"},
	{dbColumn: "SUM_WARNINGS", alias: "warnings", uiKey: "warnings", displayName: "Warnings", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Warnings"},

	// Row operations
	{dbColumn: "SUM_ROWS_AFFECTED", alias: "rows_affected", uiKey: "rowsAffected", displayName: "Rows Affected", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Rows Affected"},
	{dbColumn: "SUM_ROWS_SENT", alias: "rows_sent", uiKey: "rowsSent", displayName: "Rows Sent", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Rows Sent"},
	{dbColumn: "SUM_ROWS_EXAMINED", alias: "rows_examined", uiKey: "rowsExamined", displayName: "Rows Examined", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Rows Examined"},

	// Temp table usage
	{dbColumn: "SUM_CREATED_TMP_DISK_TABLES", alias: "tmp_disk_tables", uiKey: "tmpDiskTables", displayName: "Temp Disk Tables", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Temp Disk Tables"},
	{dbColumn: "SUM_CREATED_TMP_TABLES", alias: "tmp_tables", uiKey: "tmpTables", displayName: "Temp Tables", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Temp Tables"},

	// Join operations
	{dbColumn: "SUM_SELECT_FULL_JOIN", alias: "full_join", uiKey: "fullJoin", displayName: "Full Joins", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Full Joins"},
	{dbColumn: "SUM_SELECT_FULL_RANGE_JOIN", alias: "full_range_join", uiKey: "fullRangeJoin", displayName: "Full Range Joins", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "SUM_SELECT_RANGE", alias: "select_range", uiKey: "selectRange", displayName: "Select Range", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "SUM_SELECT_RANGE_CHECK", alias: "select_range_check", uiKey: "selectRangeCheck", displayName: "Select Range Check", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "SUM_SELECT_SCAN", alias: "select_scan", uiKey: "selectScan", displayName: "Select Scan", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Table Scans"},

	// Sort operations
	{dbColumn: "SUM_SORT_MERGE_PASSES", alias: "sort_merge_passes", uiKey: "sortMergePasses", displayName: "Sort Merge Passes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "SUM_SORT_RANGE", alias: "sort_range", uiKey: "sortRange", displayName: "Sort Range", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "SUM_SORT_ROWS", alias: "sort_rows", uiKey: "sortRows", displayName: "Sort Rows", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by Rows Sorted"},
	{dbColumn: "SUM_SORT_SCAN", alias: "sort_scan", uiKey: "sortScan", displayName: "Sort Scan", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},

	// Index usage
	{dbColumn: "SUM_NO_INDEX_USED", alias: "no_index_used", uiKey: "noIndexUsed", displayName: "No Index Used", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Top queries by No Index Used"},
	{dbColumn: "SUM_NO_GOOD_INDEX_USED", alias: "no_good_index_used", uiKey: "noGoodIndexUsed", displayName: "No Good Index Used", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},

	// Timestamp columns
	{dbColumn: "FIRST_SEEN", alias: "first_seen", uiKey: "firstSeen", displayName: "First Seen", dataType: "string", visible: false, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
	{dbColumn: "LAST_SEEN", alias: "last_seen", uiKey: "lastSeen", displayName: "Last Seen", dataType: "string", visible: false, transform: "none", sortDir: "descending", summary: "count", filter: "multiselect"},

	// MySQL 8.0+ quantile columns
	{dbColumn: "QUANTILE_95", alias: "p95_time", uiKey: "p95Time", displayName: "P95 Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by 95th Percentile Time"},
	{dbColumn: "QUANTILE_99", alias: "p99_time", uiKey: "p99Time", displayName: "P99 Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by 99th Percentile Time"},
	{dbColumn: "QUANTILE_999", alias: "p999_time", uiKey: "p999Time", displayName: "P99.9 Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isPicoseconds: true},

	// MySQL 8.0+ sample query
	{dbColumn: "QUERY_SAMPLE_TEXT", alias: "sample_query", uiKey: "sampleQuery", displayName: "Sample Query", dataType: "string", visible: false, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", fullWidth: true},
	{dbColumn: "QUERY_SAMPLE_SEEN", alias: "sample_seen", uiKey: "sampleSeen", displayName: "Sample Seen", dataType: "string", visible: false, transform: "none", sortDir: "descending", summary: "count", filter: "multiselect"},
	{dbColumn: "QUERY_SAMPLE_TIMER_WAIT", alias: "sample_time", uiKey: "sampleTime", displayName: "Sample Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isPicoseconds: true},

	// MySQL 8.0.28+ CPU time
	{dbColumn: "SUM_CPU_TIME", alias: "cpu_time", uiKey: "cpuTime", displayName: "CPU Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range", isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by CPU Time"},

	// MySQL 8.0.31+ memory columns
	{dbColumn: "MAX_CONTROLLED_MEMORY", alias: "max_controlled_memory", uiKey: "maxControlledMemory", displayName: "Max Controlled Memory", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "max", filter: "range", isSortOption: true, sortLabel: "Top queries by Max Controlled Memory"},
	{dbColumn: "MAX_TOTAL_MEMORY", alias: "max_total_memory", uiKey: "maxTotalMemory", displayName: "Max Total Memory", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "max", filter: "range", isSortOption: true, sortLabel: "Top queries by Max Total Memory"},
}

// mysqlMethods returns the available function methods for MySQL
func mysqlMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []module.SortOption
	for _, col := range mysqlAllColumns {
		if col.isSortOption {
			sortOptions = append(sortOptions, module.SortOption{
				ID:      col.uiKey,
				Column:  col.dbColumn,
				Label:   col.sortLabel,
				Default: col.isDefaultSort,
			})
		}
	}

	return []module.MethodConfig{{
		ID:          "top-queries",
		Name:        "Top Queries",
		Help:        "Top SQL queries from performance_schema",
		SortOptions: sortOptions,
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

// detectMySQLStatementsColumns queries the database to discover available columns
func (c *Collector) detectMySQLStatementsColumns(ctx context.Context) (map[string]bool, error) {
	// Fast path: return cached result
	c.stmtSummaryColsMu.RLock()
	if c.stmtSummaryCols != nil {
		cols := c.stmtSummaryCols
		c.stmtSummaryColsMu.RUnlock()
		return cols, nil
	}
	c.stmtSummaryColsMu.RUnlock()

	// Slow path: query and cache
	c.stmtSummaryColsMu.Lock()
	defer c.stmtSummaryColsMu.Unlock()

	// Double-check after acquiring write lock
	if c.stmtSummaryCols != nil {
		return c.stmtSummaryCols, nil
	}

	// Query information_schema to get available columns
	query := `
		SELECT COLUMN_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = 'performance_schema'
		AND TABLE_NAME = 'events_statements_summary_by_digest'
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

	// Cache the result
	c.stmtSummaryCols = cols

	return cols, nil
}

// buildAvailableColumns filters columns based on what's available in the database
func (c *Collector) buildAvailableMySQLColumns(availableCols map[string]bool) []mysqlColumnMeta {
	var cols []mysqlColumnMeta
	for _, col := range mysqlAllColumns {
		if availableCols[col.dbColumn] {
			cols = append(cols, col)
		}
	}
	return cols
}

// mapAndValidateMySQLSortColumn maps UI sort key to SQL alias and validates
func (c *Collector) mapAndValidateMySQLSortColumn(sortKey string, availableCols map[string]bool) string {
	// Find the column mapping from UI key or DB column to SQL alias
	for _, col := range mysqlAllColumns {
		if (col.uiKey == sortKey || col.dbColumn == sortKey || col.alias == sortKey) && availableCols[col.dbColumn] {
			return col.alias
		}
	}
	// Default to total_time (SUM_TIMER_WAIT alias) if available
	if availableCols["SUM_TIMER_WAIT"] {
		return "total_time"
	}
	return "calls" // Ultimate fallback (COUNT_STAR alias)
}

// buildMySQLDynamicSQL builds the SQL query with only available columns
func (c *Collector) buildMySQLDynamicSQL(cols []mysqlColumnMeta, sortColumn string, limit int) string {
	var selectParts []string
	for _, col := range cols {
		if col.isPicoseconds {
			// Convert picoseconds to milliseconds (divide by 10^9)
			selectParts = append(selectParts, fmt.Sprintf("%s/1000000000 AS %s", col.dbColumn, col.alias))
		} else if col.dbColumn == "SCHEMA_NAME" {
			selectParts = append(selectParts, fmt.Sprintf("IFNULL(%s, '') AS %s", col.dbColumn, col.alias))
		} else {
			selectParts = append(selectParts, fmt.Sprintf("%s AS %s", col.dbColumn, col.alias))
		}
	}

	return fmt.Sprintf(`
SELECT %s
FROM performance_schema.events_statements_summary_by_digest
WHERE DIGEST IS NOT NULL
ORDER BY %s DESC
LIMIT %d
`, strings.Join(selectParts, ", "), sortColumn, limit)
}

// scanMySQLDynamicRows scans rows dynamically based on column types
func (c *Collector) scanMySQLDynamicRows(rows mysqlRowScanner, cols []mysqlColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning
	valuePtrs := make([]any, len(cols))
	values := make([]any, len(cols))

	for rows.Next() {
		// Reset value holders for each row
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
					if col.uiKey == "query" || col.uiKey == "sampleQuery" {
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

// buildMySQLDynamicColumns builds column definitions for the response
func (c *Collector) buildMySQLDynamicColumns(cols []mysqlColumnMeta) map[string]any {
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

// mysqlRowScanner interface for testing
type mysqlRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// collectTopQueries queries performance_schema for top queries using dynamic columns
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Check if performance_schema is enabled
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

	// Detect available columns
	availableCols, err := c.detectMySQLStatementsColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	// Build list of available columns
	cols := c.buildAvailableMySQLColumns(availableCols)
	if len(cols) == 0 {
		return &module.FunctionResponse{
			Status:  500,
			Message: "no columns available in events_statements_summary_by_digest",
		}
	}

	// Validate and map sort column
	dbSortColumn := c.mapAndValidateMySQLSortColumn(sortColumn, availableCols)

	// Get query limit (default 500)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	// Build and execute query
	query := c.buildMySQLDynamicSQL(cols, dbSortColumn, limit)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	// Scan rows dynamically
	data, err := c.scanMySQLDynamicRows(rows, cols)
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
		Help:              "Top SQL queries from performance_schema.events_statements_summary_by_digest",
		Columns:           c.buildMySQLDynamicColumns(cols),
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
			"Rows": {
				Name:    "Rows",
				Type:    "stacked-bar",
				Columns: []string{"rowsSent", "rowsExamined", "rowsAffected"},
			},
			"Errors": {
				Name:    "Errors & Warnings",
				Type:    "stacked-bar",
				Columns: []string{"errors", "warnings"},
			},
		},
		DefaultCharts: [][]string{
			{"Time", "schema"},
			{"Calls", "schema"},
		},
		GroupBy: map[string]module.GroupByConfig{
			"schema": {
				Name:    "Group by Schema",
				Columns: []string{"schema"},
			},
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
