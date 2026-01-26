// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

const (
	paramSort = "__sort"

	ftString   = funcapi.FieldTypeString
	ftInteger  = funcapi.FieldTypeInteger
	ftDuration = funcapi.FieldTypeDuration

	trNone     = funcapi.FieldTransformNone
	trNumber   = funcapi.FieldTransformNumber
	trDuration = funcapi.FieldTransformDuration

	sortAsc  = funcapi.FieldSortAscending
	sortDesc = funcapi.FieldSortDescending

	summaryCount = funcapi.FieldSummaryCount
	summarySum   = funcapi.FieldSummarySum
	summaryMin   = funcapi.FieldSummaryMin
	summaryMax   = funcapi.FieldSummaryMax
	summaryMean  = funcapi.FieldSummaryMean

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

// mysqlColumnMeta defines metadata for a single column
type mysqlColumnMeta struct {
	dbColumn       string                 // Column name in database (e.g., "SUM_TIMER_WAIT")
	uiKey          string                 // Canonical name used everywhere: SQL alias, UI key, sort key
	displayName    string                 // Display name in UI (e.g., "Total Time")
	dataType       funcapi.FieldType      // "string", "integer", "float", "duration"
	units          string                 // Unit for duration/numeric types (e.g., "seconds")
	visible        bool                   // Default visibility
	transform      funcapi.FieldTransform // Transform for value_options (e.g., "duration", "number", "none")
	decimalPoints  int                    // Decimal points for display
	sortDir        funcapi.FieldSort      // Sort direction: "ascending" or "descending"
	summary        funcapi.FieldSummary   // Summary function: "sum", "count", "max", "min", "mean" (UI aggregations)
	filter         funcapi.FieldFilter    // Filter type: "multiselect" or "range"
	isPicoseconds  bool                   // Needs picoseconds to seconds conversion
	isSortOption   bool                   // Show in sort dropdown
	sortLabel      string                 // Label for sort option
	isDefaultSort  bool                   // Is this the default sort option
	isUniqueKey    bool                   // Is this column a unique key
	isSticky       bool                   // Is this column sticky in UI
	fullWidth      bool                   // Should column take full width
	isLabel        bool                   // Is this column a label for grouping
	isPrimary      bool                   // Is this label the primary grouping
	isMetric       bool                   // Is this column a chartable metric
	chartGroup     string                 // Chart group key
	chartTitle     string                 // Chart title
	isDefaultChart bool                   // Include this chart group in defaults
}

// mysqlAllColumns defines ALL possible columns from events_statements_summary_by_digest
// Columns that don't exist in certain MySQL/MariaDB versions will be filtered at runtime
var mysqlAllColumns = []mysqlColumnMeta{
	// Identity columns - always available
	{dbColumn: "DIGEST", uiKey: "digest", displayName: "Digest", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isUniqueKey: true},
	{dbColumn: "DIGEST_TEXT", uiKey: "query", displayName: "Query", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isSticky: true, fullWidth: true},
	{dbColumn: "SCHEMA_NAME", uiKey: "schema", displayName: "Schema", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},

	// Execution counts
	{dbColumn: "COUNT_STAR", uiKey: "calls", displayName: "Calls", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Number of Calls"},

	// Timer metrics (picoseconds -> seconds)
	{dbColumn: "SUM_TIMER_WAIT", uiKey: "totalTime", displayName: "Total Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by Total Execution Time", isDefaultSort: true},
	{dbColumn: "MIN_TIMER_WAIT", uiKey: "minTime", displayName: "Min Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange, isPicoseconds: true},
	{dbColumn: "AVG_TIMER_WAIT", uiKey: "avgTime", displayName: "Avg Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, filter: filterRange, isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by Average Execution Time"},
	{dbColumn: "MAX_TIMER_WAIT", uiKey: "maxTime", displayName: "Max Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isPicoseconds: true},

	// Lock time (picoseconds -> seconds)
	{dbColumn: "SUM_LOCK_TIME", uiKey: "lockTime", displayName: "Lock Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by Lock Time"},

	// Error and warning counts
	{dbColumn: "SUM_ERRORS", uiKey: "errors", displayName: "Errors", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Errors"},
	{dbColumn: "SUM_WARNINGS", uiKey: "warnings", displayName: "Warnings", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Warnings"},

	// Row operations
	{dbColumn: "SUM_ROWS_AFFECTED", uiKey: "rowsAffected", displayName: "Rows Affected", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Rows Affected"},
	{dbColumn: "SUM_ROWS_SENT", uiKey: "rowsSent", displayName: "Rows Sent", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Rows Sent"},
	{dbColumn: "SUM_ROWS_EXAMINED", uiKey: "rowsExamined", displayName: "Rows Examined", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Rows Examined"},

	// Temp table usage
	{dbColumn: "SUM_CREATED_TMP_DISK_TABLES", uiKey: "tmpDiskTables", displayName: "Temp Disk Tables", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Temp Disk Tables"},
	{dbColumn: "SUM_CREATED_TMP_TABLES", uiKey: "tmpTables", displayName: "Temp Tables", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Temp Tables"},

	// Join operations
	{dbColumn: "SUM_SELECT_FULL_JOIN", uiKey: "fullJoin", displayName: "Full Joins", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Full Joins"},
	{dbColumn: "SUM_SELECT_FULL_RANGE_JOIN", uiKey: "fullRangeJoin", displayName: "Full Range Joins", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "SUM_SELECT_RANGE", uiKey: "selectRange", displayName: "Select Range", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "SUM_SELECT_RANGE_CHECK", uiKey: "selectRangeCheck", displayName: "Select Range Check", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "SUM_SELECT_SCAN", uiKey: "selectScan", displayName: "Select Scan", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Table Scans"},

	// Sort operations
	{dbColumn: "SUM_SORT_MERGE_PASSES", uiKey: "sortMergePasses", displayName: "Sort Merge Passes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "SUM_SORT_RANGE", uiKey: "sortRange", displayName: "Sort Range", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "SUM_SORT_ROWS", uiKey: "sortRows", displayName: "Sort Rows", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Rows Sorted"},
	{dbColumn: "SUM_SORT_SCAN", uiKey: "sortScan", displayName: "Sort Scan", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// Index usage
	{dbColumn: "SUM_NO_INDEX_USED", uiKey: "noIndexUsed", displayName: "No Index Used", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by No Index Used"},
	{dbColumn: "SUM_NO_GOOD_INDEX_USED", uiKey: "noGoodIndexUsed", displayName: "No Good Index Used", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// Timestamp columns
	{dbColumn: "FIRST_SEEN", uiKey: "firstSeen", displayName: "First Seen", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},
	{dbColumn: "LAST_SEEN", uiKey: "lastSeen", displayName: "Last Seen", dataType: ftString, visible: false, transform: trNone, sortDir: sortDesc, summary: summaryCount, filter: filterMulti},

	// MySQL 8.0+ quantile columns
	{dbColumn: "QUANTILE_95", uiKey: "p95Time", displayName: "P95 Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by 95th Percentile Time"},
	{dbColumn: "QUANTILE_99", uiKey: "p99Time", displayName: "P99 Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by 99th Percentile Time"},
	{dbColumn: "QUANTILE_999", uiKey: "p999Time", displayName: "P99.9 Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isPicoseconds: true},

	// MySQL 8.0+ sample query
	{dbColumn: "QUERY_SAMPLE_TEXT", uiKey: "sampleQuery", displayName: "Sample Query", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, fullWidth: true},
	{dbColumn: "QUERY_SAMPLE_SEEN", uiKey: "sampleSeen", displayName: "Sample Seen", dataType: ftString, visible: false, transform: trNone, sortDir: sortDesc, summary: summaryCount, filter: filterMulti},
	{dbColumn: "QUERY_SAMPLE_TIMER_WAIT", uiKey: "sampleTime", displayName: "Sample Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isPicoseconds: true},

	// MySQL 8.0.28+ CPU time
	{dbColumn: "SUM_CPU_TIME", uiKey: "cpuTime", displayName: "CPU Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isPicoseconds: true, isSortOption: true, sortLabel: "Top queries by CPU Time"},

	// MySQL 8.0.31+ memory columns
	{dbColumn: "MAX_CONTROLLED_MEMORY", uiKey: "maxControlledMemory", displayName: "Max Controlled Memory", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Max Controlled Memory"},
	{dbColumn: "MAX_TOTAL_MEMORY", uiKey: "maxTotalMemory", displayName: "Max Total Memory", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Max Total Memory"},
}

type mysqlChartGroup struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}

var mysqlChartGroups = []mysqlChartGroup{
	{key: "Calls", title: "Number of Calls", columns: []string{"calls"}, defaultChart: true},
	{key: "Time", title: "Execution Time", columns: []string{"totalTime", "avgTime", "minTime", "maxTime"}, defaultChart: true},
	{key: "Percentiles", title: "Execution Time Percentiles", columns: []string{"p95Time", "p99Time", "p999Time"}},
	{key: "LockTime", title: "Lock Time", columns: []string{"lockTime"}},
	{key: "Errors", title: "Errors & Warnings", columns: []string{"errors", "warnings"}},
	{key: "Rows", title: "Rows", columns: []string{"rowsSent", "rowsExamined", "rowsAffected"}},
	{key: "TempTables", title: "Temp Tables", columns: []string{"tmpDiskTables", "tmpTables"}},
	{key: "Joins", title: "Join Operations", columns: []string{"fullJoin", "fullRangeJoin", "selectRange", "selectRangeCheck", "selectScan"}},
	{key: "Sort", title: "Sort Operations", columns: []string{"sortMergePasses", "sortRange", "sortRows", "sortScan"}},
	{key: "Index", title: "Index Usage", columns: []string{"noIndexUsed", "noGoodIndexUsed"}},
	{key: "CPU", title: "CPU Time", columns: []string{"cpuTime"}},
	{key: "Memory", title: "Memory", columns: []string{"maxControlledMemory", "maxTotalMemory"}},
	{key: "Sample", title: "Sample Time", columns: []string{"sampleTime"}},
}

var mysqlLabelColumns = map[string]bool{
	"schema": true,
}

const mysqlPrimaryLabel = "schema"

// mysqlMethods returns the available function methods for MySQL
func mysqlMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range mysqlAllColumns {
		if col.isSortOption {
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.uiKey,
				Column:  col.dbColumn,
				Name:    col.sortLabel,
				Default: col.isDefaultSort,
				Sort:    &sortDir,
			})
		}
	}

	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from performance_schema",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				{
					ID:         paramSort,
					Name:       "Filter By",
					Help:       "Select the primary sort column",
					Selection:  funcapi.ParamSelect,
					Options:    sortOptions,
					UniqueView: true,
				},
			},
		},
	}
}

func mysqlMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	if collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case "top-queries":
		return collector.topQueriesParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// mysqlHandleMethod handles function requests for MySQL
func mysqlHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
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
		return collector.collectTopQueries(ctx, params.Column(paramSort))
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

// mapAndValidateMySQLSortColumn validates sort key and returns the uiKey to use
func (c *Collector) mapAndValidateMySQLSortColumn(sortKey string, availableCols map[string]bool) string {
	// Find the column by uiKey or dbColumn
	for _, col := range mysqlAllColumns {
		if (col.uiKey == sortKey || col.dbColumn == sortKey) && availableCols[col.dbColumn] {
			return col.uiKey
		}
	}
	// Default to totalTime if available
	if availableCols["SUM_TIMER_WAIT"] {
		return "totalTime"
	}
	return "calls" // Ultimate fallback
}

// buildMySQLDynamicSQL builds the SQL query with only available columns
func (c *Collector) buildMySQLDynamicSQL(cols []mysqlColumnMeta, sortColumn string, limit int) string {
	var selectParts []string
	for _, col := range cols {
		// Use backticks to handle reserved keywords
		if col.isPicoseconds {
			// Convert picoseconds to milliseconds (divide by 10^9)
			selectParts = append(selectParts, fmt.Sprintf("%s/1000000000 AS `%s`", col.dbColumn, col.uiKey))
		} else if col.dbColumn == "SCHEMA_NAME" {
			selectParts = append(selectParts, fmt.Sprintf("IFNULL(%s, '') AS `%s`", col.dbColumn, col.uiKey))
		} else {
			selectParts = append(selectParts, fmt.Sprintf("%s AS `%s`", col.dbColumn, col.uiKey))
		}
	}

	return fmt.Sprintf(`
SELECT %s
FROM performance_schema.events_statements_summary_by_digest
WHERE DIGEST IS NOT NULL
ORDER BY `+"`%s`"+` DESC
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
			case ftString:
				var v sql.NullString
				values[i] = &v
			case ftInteger:
				var v sql.NullInt64
				values[i] = &v
			case ftDuration:
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
		visual := funcapi.FieldVisualValue
		if col.dataType == ftDuration {
			visual = funcapi.FieldVisualBar
		}
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.displayName,
			Type:                  col.dataType,
			Units:                 col.units,
			Visualization:         visual,
			Sort:                  col.sortDir,
			Sortable:              true,
			Sticky:                col.isSticky,
			Summary:               col.summary,
			Filter:                col.filter,
			FullWidth:             col.fullWidth,
			Wrap:                  false,
			DefaultExpandedFilter: false,
			UniqueKey:             col.isUniqueKey,
			Visible:               col.visible,
			ValueOptions: funcapi.ValueOptions{
				Transform:     col.transform,
				DecimalPoints: col.decimalPoints,
				DefaultValue:  nil,
			},
		}
		columns[col.uiKey] = colDef.BuildColumn()
	}
	return columns
}

// buildMySQLDynamicSortOptions builds sort options from available columns
// Returns only sort options for columns that actually exist in the database
func (c *Collector) buildMySQLDynamicSortOptions(cols []mysqlColumnMeta) []funcapi.ParamOption {
	var sortOpts []funcapi.ParamOption
	seen := make(map[string]bool)
	sortDir := funcapi.FieldSortDescending

	for _, col := range cols {
		if col.isSortOption && !seen[col.uiKey] {
			seen[col.uiKey] = true
			sortOpts = append(sortOpts, funcapi.ParamOption{
				ID:      col.uiKey,
				Column:  col.dbColumn,
				Name:    col.sortLabel,
				Default: col.isDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return sortOpts
}

func (c *Collector) topQueriesSortParam(cols []mysqlColumnMeta) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := c.buildMySQLDynamicSortOptions(cols)
	sortParam := funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    sortOptions,
		UniqueView: true,
	}
	return sortParam, sortOptions
}

func (c *Collector) topQueriesParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	available, err := c.checkPerformanceSchema(ctx)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, fmt.Errorf("performance_schema is not enabled")
	}

	availableCols, err := c.detectMySQLStatementsColumns(ctx)
	if err != nil {
		return nil, err
	}
	cols := c.buildAvailableMySQLColumns(availableCols)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in events_statements_summary_by_digest")
	}

	sortParam, _ := c.topQueriesSortParam(cols)
	return []funcapi.ParamConfig{sortParam}, nil
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

	// Build dynamic sort options from available columns (only those actually detected)
	sortParam, sortOptions := c.topQueriesSortParam(cols)

	// Find default sort column UI key
	defaultSort := ""
	for _, col := range cols {
		if col.isDefaultSort && col.isSortOption {
			defaultSort = col.uiKey
			break
		}
	}
	// Fallback to first sort option if no default
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	annotatedCols := decorateMySQLColumns(cols)

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from performance_schema.events_statements_summary_by_digest",
		Columns:           c.buildMySQLDynamicColumns(cols),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},

		// Charts for aggregated visualization
		Charts:        mysqlTopQueriesCharts(annotatedCols),
		DefaultCharts: mysqlTopQueriesDefaultCharts(annotatedCols),
		GroupBy:       mysqlTopQueriesGroupBy(annotatedCols),
	}
}

func decorateMySQLColumns(cols []mysqlColumnMeta) []mysqlColumnMeta {
	out := make([]mysqlColumnMeta, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.uiKey] = i
	}

	for i := range out {
		if mysqlLabelColumns[out[i].uiKey] {
			out[i].isLabel = true
			if out[i].uiKey == mysqlPrimaryLabel {
				out[i].isPrimary = true
			}
		}
	}

	for _, group := range mysqlChartGroups {
		for _, key := range group.columns {
			idx, ok := index[key]
			if !ok {
				continue
			}
			out[idx].isMetric = true
			out[idx].chartGroup = group.key
			out[idx].chartTitle = group.title
			if group.defaultChart {
				out[idx].isDefaultChart = true
			}
		}
	}

	return out
}

func mysqlTopQueriesCharts(cols []mysqlColumnMeta) map[string]module.ChartConfig {
	charts := make(map[string]module.ChartConfig)
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" {
			continue
		}
		cfg, ok := charts[col.chartGroup]
		if !ok {
			title := col.chartTitle
			if title == "" {
				title = col.chartGroup
			}
			cfg = module.ChartConfig{Name: title, Type: "stacked-bar"}
		}
		cfg.Columns = append(cfg.Columns, col.uiKey)
		charts[col.chartGroup] = cfg
	}
	return charts
}

func mysqlTopQueriesDefaultCharts(cols []mysqlColumnMeta) [][]string {
	label := primaryMySQLLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultMySQLChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func mysqlTopQueriesGroupBy(cols []mysqlColumnMeta) map[string]module.GroupByConfig {
	groupBy := make(map[string]module.GroupByConfig)
	for _, col := range cols {
		if !col.isLabel {
			continue
		}
		groupBy[col.uiKey] = module.GroupByConfig{
			Name:    "Group by " + col.displayName,
			Columns: []string{col.uiKey},
		}
	}
	return groupBy
}

func primaryMySQLLabel(cols []mysqlColumnMeta) string {
	for _, col := range cols {
		if col.isPrimary {
			return col.uiKey
		}
	}
	for _, col := range cols {
		if col.isLabel {
			return col.uiKey
		}
	}
	return ""
}

func defaultMySQLChartGroups(cols []mysqlColumnMeta) []string {
	groups := make([]string, 0)
	seen := make(map[string]bool)
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" || !col.isDefaultChart {
			continue
		}
		if !seen[col.chartGroup] {
			seen[col.chartGroup] = true
			groups = append(groups, col.chartGroup)
		}
	}
	if len(groups) > 0 {
		return groups
	}
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" {
			continue
		}
		if !seen[col.chartGroup] {
			seen[col.chartGroup] = true
			groups = append(groups, col.chartGroup)
		}
	}
	return groups
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
