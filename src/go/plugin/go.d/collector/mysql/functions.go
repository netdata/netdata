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

const paramSort = "__sort"

// mysqlColumn defines a column for MySQL top-queries function.
// Embeds funcapi.ColumnMeta for UI display and adds collector-specific fields.
type mysqlColumn struct {
	funcapi.ColumnMeta

	// Data access
	DBColumn      string // Column name in database (e.g., "SUM_TIMER_WAIT")
	IsPicoseconds bool   // Needs picoseconds to milliseconds conversion

	// Sort parameter metadata
	IsSortOption  bool   // Show in sort dropdown
	SortLabel     string // Label for sort option
	IsDefaultSort bool   // Is this the default sort option
}

// mysqlColumnSet creates a ColumnSet for the given columns.
func mysqlColumnSet(cols []mysqlColumn) funcapi.ColumnSet[mysqlColumn] {
	return funcapi.Columns(cols, func(c mysqlColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

// mysqlAllColumns defines ALL possible columns from events_statements_summary_by_digest
// Columns that don't exist in certain MySQL/MariaDB versions will be filtered at runtime
var mysqlAllColumns = []mysqlColumn{
	// Identity columns - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "digest", Tooltip: "Digest", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true, Sortable: true}, DBColumn: "DIGEST"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true, Sortable: true}, DBColumn: "DIGEST_TEXT"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "schema", Tooltip: "Schema", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "SCHEMA_NAME"},

	// Execution counts
	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "COUNT_STAR", IsSortOption: true, SortLabel: "Top queries by Number of Calls"},

	// Timer metrics (picoseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_TIMER_WAIT", IsPicoseconds: true, IsSortOption: true, SortLabel: "Top queries by Total Execution Time", IsDefaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MIN_TIMER_WAIT", IsPicoseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "AVG_TIMER_WAIT", IsPicoseconds: true, IsSortOption: true, SortLabel: "Top queries by Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MAX_TIMER_WAIT", IsPicoseconds: true},

	// Lock time (picoseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "lockTime", Tooltip: "Lock Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_LOCK_TIME", IsPicoseconds: true, IsSortOption: true, SortLabel: "Top queries by Lock Time"},

	// Error and warning counts
	{ColumnMeta: funcapi.ColumnMeta{Name: "errors", Tooltip: "Errors", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ERRORS", IsSortOption: true, SortLabel: "Top queries by Errors"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "warnings", Tooltip: "Warnings", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_WARNINGS", IsSortOption: true, SortLabel: "Top queries by Warnings"},

	// Row operations
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsAffected", Tooltip: "Rows Affected", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ROWS_AFFECTED", IsSortOption: true, SortLabel: "Top queries by Rows Affected"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsSent", Tooltip: "Rows Sent", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ROWS_SENT", IsSortOption: true, SortLabel: "Top queries by Rows Sent"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsExamined", Tooltip: "Rows Examined", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ROWS_EXAMINED", IsSortOption: true, SortLabel: "Top queries by Rows Examined"},

	// Temp table usage
	{ColumnMeta: funcapi.ColumnMeta{Name: "tmpDiskTables", Tooltip: "Temp Disk Tables", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_CREATED_TMP_DISK_TABLES", IsSortOption: true, SortLabel: "Top queries by Temp Disk Tables"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "tmpTables", Tooltip: "Temp Tables", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_CREATED_TMP_TABLES", IsSortOption: true, SortLabel: "Top queries by Temp Tables"},

	// Join operations
	{ColumnMeta: funcapi.ColumnMeta{Name: "fullJoin", Tooltip: "Full Joins", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_FULL_JOIN", IsSortOption: true, SortLabel: "Top queries by Full Joins"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "fullRangeJoin", Tooltip: "Full Range Joins", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_FULL_RANGE_JOIN"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "selectRange", Tooltip: "Select Range", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_RANGE"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "selectRangeCheck", Tooltip: "Select Range Check", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_RANGE_CHECK"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "selectScan", Tooltip: "Select Scan", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_SCAN", IsSortOption: true, SortLabel: "Top queries by Table Scans"},

	// Sort operations
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortMergePasses", Tooltip: "Sort Merge Passes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_MERGE_PASSES"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortRange", Tooltip: "Sort Range", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_RANGE"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortRows", Tooltip: "Sort Rows", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_ROWS", IsSortOption: true, SortLabel: "Top queries by Rows Sorted"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortScan", Tooltip: "Sort Scan", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_SCAN"},

	// Index usage
	{ColumnMeta: funcapi.ColumnMeta{Name: "noIndexUsed", Tooltip: "No Index Used", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_NO_INDEX_USED", IsSortOption: true, SortLabel: "Top queries by No Index Used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "noGoodIndexUsed", Tooltip: "No Good Index Used", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_NO_GOOD_INDEX_USED"},

	// Timestamp columns
	{ColumnMeta: funcapi.ColumnMeta{Name: "firstSeen", Tooltip: "First Seen", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "FIRST_SEEN"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastSeen", Tooltip: "Last Seen", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "LAST_SEEN"},

	// MySQL 8.0+ quantile columns
	{ColumnMeta: funcapi.ColumnMeta{Name: "p95Time", Tooltip: "P95 Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUANTILE_95", IsPicoseconds: true, IsSortOption: true, SortLabel: "Top queries by 95th Percentile Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "p99Time", Tooltip: "P99 Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUANTILE_99", IsPicoseconds: true, IsSortOption: true, SortLabel: "Top queries by 99th Percentile Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "p999Time", Tooltip: "P99.9 Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUANTILE_999", IsPicoseconds: true},

	// MySQL 8.0+ sample query
	{ColumnMeta: funcapi.ColumnMeta{Name: "sampleQuery", Tooltip: "Sample Query", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Sortable: true}, DBColumn: "QUERY_SAMPLE_TEXT"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sampleSeen", Tooltip: "Sample Seen", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "QUERY_SAMPLE_SEEN"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sampleTime", Tooltip: "Sample Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUERY_SAMPLE_TIMER_WAIT", IsPicoseconds: true},

	// MySQL 8.0.28+ CPU time
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpuTime", Tooltip: "CPU Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_CPU_TIME", IsPicoseconds: true, IsSortOption: true, SortLabel: "Top queries by CPU Time"},

	// MySQL 8.0.31+ memory columns
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxControlledMemory", Tooltip: "Max Controlled Memory", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MAX_CONTROLLED_MEMORY", IsSortOption: true, SortLabel: "Top queries by Max Controlled Memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTotalMemory", Tooltip: "Max Total Memory", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MAX_TOTAL_MEMORY", IsSortOption: true, SortLabel: "Top queries by Max Total Memory"},
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
	sortOptions := buildMySQLSortOptions(mysqlAllColumns)

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

// funcMysql implements funcapi.MethodHandler for MySQL.
type funcMysql struct {
	collector *Collector
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcMysql)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcMysql) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case "top-queries":
		return f.collector.topQueriesParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcMysql) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.collector.db == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	switch method {
	case "top-queries":
		return f.collector.collectTopQueries(ctx, params.Column(paramSort))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func mysqlFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return &funcMysql{collector: c}
}

func buildMySQLSortOptions(cols []mysqlColumn) []funcapi.ParamOption {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if col.IsSortOption {
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.DBColumn,
				Name:    col.SortLabel,
				Default: col.IsDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return sortOptions
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
func (c *Collector) buildAvailableMySQLColumns(availableCols map[string]bool) []mysqlColumn {
	var cols []mysqlColumn
	for _, col := range mysqlAllColumns {
		if availableCols[col.DBColumn] {
			cols = append(cols, col)
		}
	}
	return cols
}

// mapAndValidateMySQLSortColumn validates sort key and returns the ID to use
func (c *Collector) mapAndValidateMySQLSortColumn(sortKey string, availableCols map[string]bool) string {
	// Find the column by ID or DBColumn
	for _, col := range mysqlAllColumns {
		if (col.Name == sortKey || col.DBColumn == sortKey) && availableCols[col.DBColumn] {
			return col.Name
		}
	}
	// Default to totalTime if available
	if availableCols["SUM_TIMER_WAIT"] {
		return "totalTime"
	}
	return "calls" // Ultimate fallback
}

// buildMySQLDynamicSQL builds the SQL query with only available columns
func (c *Collector) buildMySQLDynamicSQL(cols []mysqlColumn, sortColumn string, limit int) string {
	var selectParts []string
	for _, col := range cols {
		// Use backticks to handle reserved keywords
		if col.IsPicoseconds {
			// Convert picoseconds to milliseconds (divide by 10^9)
			selectParts = append(selectParts, fmt.Sprintf("%s/1000000000 AS `%s`", col.DBColumn, col.Name))
		} else if col.DBColumn == "SCHEMA_NAME" {
			selectParts = append(selectParts, fmt.Sprintf("IFNULL(%s, '') AS `%s`", col.DBColumn, col.Name))
		} else {
			selectParts = append(selectParts, fmt.Sprintf("%s AS `%s`", col.DBColumn, col.Name))
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
func (c *Collector) scanMySQLDynamicRows(rows mysqlRowScanner, cols []mysqlColumn) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning
	valuePtrs := make([]any, len(cols))
	values := make([]any, len(cols))

	for rows.Next() {
		// Reset value holders for each row
		for i, col := range cols {
			switch col.Type {
			case funcapi.FieldTypeString:
				var v sql.NullString
				values[i] = &v
			case funcapi.FieldTypeInteger:
				var v sql.NullInt64
				values[i] = &v
			case funcapi.FieldTypeDuration:
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
					if col.Name == "query" || col.Name == "sampleQuery" {
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

func (c *Collector) topQueriesSortParam(cols []mysqlColumn) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := buildMySQLSortOptions(cols)
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

	// Find default sort column ID
	defaultSort := ""
	for _, col := range cols {
		if col.IsDefaultSort && col.IsSortOption {
			defaultSort = col.Name
			break
		}
	}
	// Fallback to first sort option if no default
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	// Decorate columns with chart/label metadata and create ColumnSet
	annotatedCols := decorateMySQLColumns(cols)
	cs := mysqlColumnSet(annotatedCols)

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from performance_schema.events_statements_summary_by_digest",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},

		// Charts for aggregated visualization
		Charts:        cs.BuildCharts(),
		DefaultCharts: cs.BuildDefaultCharts(),
		GroupBy:       cs.BuildGroupBy(),
	}
}

func decorateMySQLColumns(cols []mysqlColumn) []mysqlColumn {
	out := make([]mysqlColumn, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.Name] = i
	}

	for i := range out {
		if mysqlLabelColumns[out[i].Name] {
			out[i].GroupBy = &funcapi.GroupByOptions{
				IsDefault: out[i].Name == mysqlPrimaryLabel,
			}
		}
	}

	for _, group := range mysqlChartGroups {
		for _, key := range group.columns {
			idx, ok := index[key]
			if !ok {
				continue
			}
			out[idx].Chart = &funcapi.ChartOptions{
				Group:     group.key,
				Title:     group.title,
				IsDefault: group.defaultChart,
			}
		}
	}

	return out
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
