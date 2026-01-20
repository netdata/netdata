// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// pgColumnMeta defines metadata for a pg_stat_statements column
type pgColumnMeta struct {
	// Database column name (may vary by version)
	dbColumn string
	// Canonical name used everywhere: SQL alias, UI key, sort key
	uiKey string
	// Display name in UI
	displayName string
	// Data type: "string", "integer", "float", "duration"
	dataType string
	// Unit for duration/numeric types
	units string
	// Whether visible by default
	visible bool
	// Transform for value_options
	transform string
	// Decimal points for display
	decimalPoints int
	// Sort direction preference
	sortDir string
	// Summary function
	summary string
	// Filter type
	filter string
	// Whether this is a sortable option for the sort dropdown
	isSortOption bool
	// Sort option label (if isSortOption)
	sortLabel string
	// Whether this is the default sort
	isDefaultSort bool
	// Whether this is the unique key
	isUniqueKey bool
	// Whether this column is sticky (stays visible when scrolling)
	isSticky bool
	// Whether this column should take full width
	fullWidth bool
}

// pgAllColumns defines ALL possible columns from pg_stat_statements
// Order matters - this determines column index in the response
var pgAllColumns = []pgColumnMeta{
	// Core identification columns (always present)
	{dbColumn: "s.queryid::text", uiKey: "queryid", displayName: "Query ID", dataType: "string", visible: false, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isUniqueKey: true},
	{dbColumn: "s.query", uiKey: "query", displayName: "Query", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect", isSticky: true, fullWidth: true},
	{dbColumn: "d.datname", uiKey: "database", displayName: "Database", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
	{dbColumn: "u.usename", uiKey: "user", displayName: "User", dataType: "string", visible: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},

	// Execution count (always present)
	{dbColumn: "s.calls", uiKey: "calls", displayName: "Calls", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Number of Calls"},

	// Execution time columns (names vary by version - detected dynamically)
	// PG <13: total_time, mean_time, min_time, max_time, stddev_time
	// PG 13+: total_exec_time, mean_exec_time, min_exec_time, max_exec_time, stddev_exec_time
	{dbColumn: "total_time", uiKey: "totalTime", displayName: "Total Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Total Execution Time", isDefaultSort: true},
	{dbColumn: "mean_time", uiKey: "meanTime", displayName: "Mean Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range", isSortOption: true, sortLabel: "Average Execution Time"},
	{dbColumn: "min_time", uiKey: "minTime", displayName: "Min Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_time", uiKey: "maxTime", displayName: "Max Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stddev_time", uiKey: "stddevTime", displayName: "Stddev Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range"},

	// Planning time columns (PG 13+ only)
	{dbColumn: "s.plans", uiKey: "plans", displayName: "Plans", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "total_plan_time", uiKey: "totalPlanTime", displayName: "Total Plan Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "mean_plan_time", uiKey: "meanPlanTime", displayName: "Mean Plan Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "min_plan_time", uiKey: "minPlanTime", displayName: "Min Plan Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "min", filter: "range"},
	{dbColumn: "max_plan_time", uiKey: "maxPlanTime", displayName: "Max Plan Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range"},
	{dbColumn: "stddev_plan_time", uiKey: "stddevPlanTime", displayName: "Stddev Plan Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "max", filter: "range"},

	// Row count (always present)
	{dbColumn: "s.rows", uiKey: "rows", displayName: "Rows", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Rows Returned"},

	// Shared buffer statistics (always present)
	{dbColumn: "s.shared_blks_hit", uiKey: "sharedBlksHit", displayName: "Shared Blocks Hit", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Shared Blocks Hit (Cache)"},
	{dbColumn: "s.shared_blks_read", uiKey: "sharedBlksRead", displayName: "Shared Blocks Read", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Shared Blocks Read (Disk I/O)"},
	{dbColumn: "s.shared_blks_dirtied", uiKey: "sharedBlksDirtied", displayName: "Shared Blocks Dirtied", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.shared_blks_written", uiKey: "sharedBlksWritten", displayName: "Shared Blocks Written", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},

	// Local buffer statistics (always present)
	{dbColumn: "s.local_blks_hit", uiKey: "localBlksHit", displayName: "Local Blocks Hit", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.local_blks_read", uiKey: "localBlksRead", displayName: "Local Blocks Read", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.local_blks_dirtied", uiKey: "localBlksDirtied", displayName: "Local Blocks Dirtied", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.local_blks_written", uiKey: "localBlksWritten", displayName: "Local Blocks Written", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},

	// Temp buffer statistics (always present)
	{dbColumn: "s.temp_blks_read", uiKey: "tempBlksRead", displayName: "Temp Blocks Read", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.temp_blks_written", uiKey: "tempBlksWritten", displayName: "Temp Blocks Written", dataType: "integer", visible: true, transform: "number", sortDir: "descending", summary: "sum", filter: "range", isSortOption: true, sortLabel: "Temp Blocks Written"},

	// I/O timing (requires track_io_timing, always present but may be 0)
	{dbColumn: "s.blk_read_time", uiKey: "blkReadTime", displayName: "Block Read Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.blk_write_time", uiKey: "blkWriteTime", displayName: "Block Write Time", dataType: "duration", units: "milliseconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},

	// WAL statistics (PG 13+ only)
	{dbColumn: "s.wal_records", uiKey: "walRecords", displayName: "WAL Records", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.wal_fpi", uiKey: "walFpi", displayName: "WAL Full Page Images", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.wal_bytes", uiKey: "walBytes", displayName: "WAL Bytes", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},

	// JIT statistics (PG 15+ only)
	{dbColumn: "s.jit_functions", uiKey: "jitFunctions", displayName: "JIT Functions", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_generation_time", uiKey: "jitGenerationTime", displayName: "JIT Generation Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_inlining_count", uiKey: "jitInliningCount", displayName: "JIT Inlining Count", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_inlining_time", uiKey: "jitInliningTime", displayName: "JIT Inlining Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_optimization_count", uiKey: "jitOptimizationCount", displayName: "JIT Optimization Count", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_optimization_time", uiKey: "jitOptimizationTime", displayName: "JIT Optimization Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_emission_count", uiKey: "jitEmissionCount", displayName: "JIT Emission Count", dataType: "integer", visible: false, transform: "number", sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.jit_emission_time", uiKey: "jitEmissionTime", displayName: "JIT Emission Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},

	// Temp file statistics (PG 15+ only)
	{dbColumn: "s.temp_blk_read_time", uiKey: "tempBlkReadTime", displayName: "Temp Block Read Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	{dbColumn: "s.temp_blk_write_time", uiKey: "tempBlkWriteTime", displayName: "Temp Block Write Time", dataType: "duration", units: "milliseconds", visible: false, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
}

// pgMethods returns the available function methods for PostgreSQL
// Sort options are built dynamically based on available columns
func pgMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []module.SortOption
	for _, col := range pgAllColumns {
		if col.isSortOption {
			sortOptions = append(sortOptions, module.SortOption{
				ID:      col.uiKey,
				Column:  col.dbColumn,
				Label:   "Top queries by " + col.sortLabel,
				Default: col.isDefaultSort,
			})
		}
	}

	return []module.MethodConfig{{
		ID:          "top-queries",
		Name:        "Top Queries",
		Help:        "Top SQL queries from pg_stat_statements",
		SortOptions: sortOptions,
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
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
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

	// Detect available columns (lazy detection, cached)
	availableCols, err := c.detectPgStatStatementsColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	// Build list of columns to query based on what's available
	queryCols := c.buildAvailableColumns(availableCols)
	if len(queryCols) == 0 {
		return &module.FunctionResponse{
			Status:  500,
			Message: "no queryable columns found in pg_stat_statements",
		}
	}

	// Map and validate sort column
	actualSortCol := c.mapAndValidateSortColumn(sortColumn, availableCols)

	// Get query limit (default 500)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	// Build and execute query
	query := c.buildDynamicSQL(queryCols, actualSortCol, limit)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	// Process rows and build response
	data, err := c.scanDynamicRows(rows, queryCols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	if err := rows.Err(); err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("rows iteration error: %v", err)}
	}

	// Find default sort column UI key from metadata
	defaultSort := "totalTime"
	for _, col := range queryCols {
		if col.isDefaultSort {
			defaultSort = col.uiKey
			break
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements",
		Columns:           c.buildDynamicColumns(queryCols),
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
				Columns: []string{"totalTime", "meanTime"},
			},
			"Rows": {
				Name:    "Rows Returned",
				Type:    "stacked-bar",
				Columns: []string{"rows"},
			},
			"IO": {
				Name:    "Block I/O",
				Type:    "stacked-bar",
				Columns: []string{"sharedBlksHit", "sharedBlksRead"},
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
			"user": {
				Name:    "Group by User",
				Columns: []string{"user"},
			},
		},
	}
}

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

// buildAvailableColumns returns column metadata for columns that exist in this PG version
func (c *Collector) buildAvailableColumns(availableCols map[string]bool) []pgColumnMeta {
	var result []pgColumnMeta

	for _, col := range pgAllColumns {
		// Extract the actual column name (remove table prefix and type cast)
		colName := col.dbColumn
		if idx := strings.LastIndex(colName, "."); idx != -1 {
			colName = colName[idx+1:]
		}
		// Remove PostgreSQL type cast suffix (e.g., "::text")
		if idx := strings.Index(colName, "::"); idx != -1 {
			colName = colName[:idx]
		}

		// Handle version-specific column names for time columns
		// PG 13+ renamed time columns: total_time -> total_exec_time, etc.
		actualColName := colName
		if c.pgVersion >= pgVersion13 {
			switch colName {
			case "total_time":
				actualColName = "total_exec_time"
			case "mean_time":
				actualColName = "mean_exec_time"
			case "min_time":
				actualColName = "min_exec_time"
			case "max_time":
				actualColName = "max_exec_time"
			case "stddev_time":
				actualColName = "stddev_exec_time"
			}
		}

		// Check if column exists (either directly or via join)
		// Join columns (database, user) come from other tables (d.datname, u.usename)
		isJoinCol := col.uiKey == "database" || col.uiKey == "user"
		if isJoinCol || availableCols[actualColName] {
			// Create a copy with the actual column name for this version
			colCopy := col
			if actualColName != colName {
				// Update dbColumn to use the version-specific name with alias
				prefix := "s."
				if strings.HasPrefix(col.dbColumn, "s.") {
					prefix = ""
					colCopy.dbColumn = "s." + actualColName
				}
				_ = prefix // suppress unused warning
			}
			result = append(result, colCopy)
		}
	}

	return result
}

// mapAndValidateSortColumn maps the semantic sort column to actual SQL column
func (c *Collector) mapAndValidateSortColumn(sortColumn string, availableCols map[string]bool) string {
	// Map UI key back to dbColumn
	for _, col := range pgAllColumns {
		if col.uiKey == sortColumn || col.dbColumn == sortColumn {
			// Get actual column name (strip table prefix and type cast)
			colName := col.dbColumn
			if idx := strings.LastIndex(colName, "."); idx != -1 {
				colName = colName[idx+1:]
			}
			if idx := strings.Index(colName, "::"); idx != -1 {
				colName = colName[:idx]
			}

			// Handle version-specific mapping
			if c.pgVersion >= pgVersion13 {
				switch colName {
				case "total_time":
					colName = "total_exec_time"
				case "mean_time":
					colName = "mean_exec_time"
				case "min_time":
					colName = "min_exec_time"
				case "max_time":
					colName = "max_exec_time"
				case "stddev_time":
					colName = "stddev_exec_time"
				}
			}

			// Validate column exists
			if availableCols[colName] {
				return colName
			}
		}
	}

	// Default fallback
	if c.pgVersion >= pgVersion13 {
		return "total_exec_time"
	}
	return "total_time"
}

// buildDynamicSQL builds the SQL query with only available columns
func (c *Collector) buildDynamicSQL(cols []pgColumnMeta, sortColumn string, limit int) string {
	var selectCols []string

	for _, col := range cols {
		colExpr := col.dbColumn

		// Handle version-specific column names
		if c.pgVersion >= pgVersion13 {
			switch {
			case strings.HasSuffix(colExpr, ".total_time"):
				colExpr = strings.Replace(colExpr, ".total_time", ".total_exec_time", 1)
			case strings.HasSuffix(colExpr, ".mean_time"):
				colExpr = strings.Replace(colExpr, ".mean_time", ".mean_exec_time", 1)
			case strings.HasSuffix(colExpr, ".min_time"):
				colExpr = strings.Replace(colExpr, ".min_time", ".min_exec_time", 1)
			case strings.HasSuffix(colExpr, ".max_time"):
				colExpr = strings.Replace(colExpr, ".max_time", ".max_exec_time", 1)
			case strings.HasSuffix(colExpr, ".stddev_time"):
				colExpr = strings.Replace(colExpr, ".stddev_time", ".stddev_exec_time", 1)
			case colExpr == "total_time":
				colExpr = "total_exec_time"
			case colExpr == "mean_time":
				colExpr = "mean_exec_time"
			case colExpr == "min_time":
				colExpr = "min_exec_time"
			case colExpr == "max_time":
				colExpr = "max_exec_time"
			case colExpr == "stddev_time":
				colExpr = "stddev_exec_time"
			}
		}

		// Always use uiKey as the SQL alias for consistent naming
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", colExpr, col.uiKey))
	}

	return fmt.Sprintf(`
SELECT %s
FROM pg_stat_statements s
JOIN pg_database d ON s.dbid = d.oid
JOIN pg_user u ON s.userid = u.usesysid
ORDER BY %s DESC
LIMIT %d
`, strings.Join(selectCols, ", "), sortColumn, limit)
}

// scanDynamicRows scans rows into the data array based on column types
// Uses sql.Null* types to handle NULL values safely
func (c *Collector) scanDynamicRows(rows dbRows, cols []pgColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning (reuse across rows for efficiency)
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
				var v sql.NullString
				values[i] = &v
			}
			valuePtrs[i] = values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("row scan failed: %v", err)
		}

		// Convert scanned values to output format
		row := make([]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case *sql.NullString:
				if v.Valid {
					s := v.String
					if col.uiKey == "query" {
						row[i] = strmutil.TruncateText(s, maxQueryTextLength)
					} else {
						row[i] = s
					}
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
			}
		}

		data = append(data, row)
	}

	return data, nil
}

// buildDynamicColumns builds column definitions for the response
func (c *Collector) buildDynamicColumns(cols []pgColumnMeta) map[string]any {
	result := make(map[string]any)

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

		// Use bar visualization for duration columns
		if col.dataType == "duration" {
			colDef["visualization"] = "bar"
		}

		result[col.uiKey] = colDef
	}

	return result
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

// dbRows interface for testing
type dbRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}
