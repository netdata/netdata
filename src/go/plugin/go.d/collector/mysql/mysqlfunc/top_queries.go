// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	topQueriesMethodID      = "top-queries"
	topQueriesMaxTextLength = 4096
	topQueriesParamSort     = "__sort"
)

func topQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Top SQL queries from performance_schema",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)},
	}
}

// topQueriesColumn defines a column for MySQL top-queries function.
// Embeds funcapi.ColumnMeta for UI display and adds collector-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta

	// Data access
	DBColumn      string // Column name in database (e.g., "SUM_TIMER_WAIT")
	IsPicoseconds bool   // Needs picoseconds to milliseconds conversion

	// Sort parameter metadata
	sortOpt     bool   // Show in sort dropdown
	sortLbl     string // Label for sort option
	defaultSort bool   // Is this the default sort option
}

// funcapi.SortableColumn interface implementation for topQueriesColumn.
func (c topQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c topQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c topQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c topQueriesColumn) ColumnName() string  { return c.Name }
func (c topQueriesColumn) SortColumn() string  { return "" }

// topQueriesColumns defines ALL possible columns from events_statements_summary_by_digest
// Columns that don't exist in certain MySQL/MariaDB versions will be filtered at runtime
var topQueriesColumns = []topQueriesColumn{
	// Identity columns - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "digest", Tooltip: "Digest", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true, Sortable: true}, DBColumn: "DIGEST"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true, Sortable: true}, DBColumn: "DIGEST_TEXT"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "schema", Tooltip: "Schema", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "SCHEMA_NAME"},

	// Execution counts
	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "COUNT_STAR", sortOpt: true, sortLbl: "Top queries by Number of Calls"},

	// Timer metrics (picoseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_TIMER_WAIT", IsPicoseconds: true, sortOpt: true, sortLbl: "Top queries by Total Execution Time", defaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MIN_TIMER_WAIT", IsPicoseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "AVG_TIMER_WAIT", IsPicoseconds: true, sortOpt: true, sortLbl: "Top queries by Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MAX_TIMER_WAIT", IsPicoseconds: true},

	// Lock time (picoseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "lockTime", Tooltip: "Lock Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_LOCK_TIME", IsPicoseconds: true, sortOpt: true, sortLbl: "Top queries by Lock Time"},

	// Error and warning counts
	{ColumnMeta: funcapi.ColumnMeta{Name: "errors", Tooltip: "Errors", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ERRORS", sortOpt: true, sortLbl: "Top queries by Errors"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "warnings", Tooltip: "Warnings", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_WARNINGS", sortOpt: true, sortLbl: "Top queries by Warnings"},

	// Row operations
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsAffected", Tooltip: "Rows Affected", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ROWS_AFFECTED", sortOpt: true, sortLbl: "Top queries by Rows Affected"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsSent", Tooltip: "Rows Sent", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ROWS_SENT", sortOpt: true, sortLbl: "Top queries by Rows Sent"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsExamined", Tooltip: "Rows Examined", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_ROWS_EXAMINED", sortOpt: true, sortLbl: "Top queries by Rows Examined"},

	// Temp table usage
	{ColumnMeta: funcapi.ColumnMeta{Name: "tmpDiskTables", Tooltip: "Temp Disk Tables", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_CREATED_TMP_DISK_TABLES", sortOpt: true, sortLbl: "Top queries by Temp Disk Tables"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "tmpTables", Tooltip: "Temp Tables", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_CREATED_TMP_TABLES", sortOpt: true, sortLbl: "Top queries by Temp Tables"},

	// Join operations
	{ColumnMeta: funcapi.ColumnMeta{Name: "fullJoin", Tooltip: "Full Joins", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_FULL_JOIN", sortOpt: true, sortLbl: "Top queries by Full Joins"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "fullRangeJoin", Tooltip: "Full Range Joins", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_FULL_RANGE_JOIN"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "selectRange", Tooltip: "Select Range", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_RANGE"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "selectRangeCheck", Tooltip: "Select Range Check", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_RANGE_CHECK"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "selectScan", Tooltip: "Select Scan", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SELECT_SCAN", sortOpt: true, sortLbl: "Top queries by Table Scans"},

	// Sort operations
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortMergePasses", Tooltip: "Sort Merge Passes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_MERGE_PASSES"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortRange", Tooltip: "Sort Range", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_RANGE"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortRows", Tooltip: "Sort Rows", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_ROWS", sortOpt: true, sortLbl: "Top queries by Rows Sorted"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sortScan", Tooltip: "Sort Scan", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_SORT_SCAN"},

	// Index usage
	{ColumnMeta: funcapi.ColumnMeta{Name: "noIndexUsed", Tooltip: "No Index Used", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_NO_INDEX_USED", sortOpt: true, sortLbl: "Top queries by No Index Used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "noGoodIndexUsed", Tooltip: "No Good Index Used", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_NO_GOOD_INDEX_USED"},

	// Timestamp columns
	{ColumnMeta: funcapi.ColumnMeta{Name: "firstSeen", Tooltip: "First Seen", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "FIRST_SEEN"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastSeen", Tooltip: "Last Seen", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "LAST_SEEN"},

	// MySQL 8.0+ quantile columns
	{ColumnMeta: funcapi.ColumnMeta{Name: "p95Time", Tooltip: "P95 Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUANTILE_95", IsPicoseconds: true, sortOpt: true, sortLbl: "Top queries by 95th Percentile Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "p99Time", Tooltip: "P99 Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUANTILE_99", IsPicoseconds: true, sortOpt: true, sortLbl: "Top queries by 99th Percentile Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "p999Time", Tooltip: "P99.9 Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUANTILE_999", IsPicoseconds: true},

	// MySQL 8.0+ sample query
	{ColumnMeta: funcapi.ColumnMeta{Name: "sampleQuery", Tooltip: "Sample Query", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, FullWidth: true, Sortable: true}, DBColumn: "QUERY_SAMPLE_TEXT"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sampleSeen", Tooltip: "Sample Seen", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "QUERY_SAMPLE_SEEN"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sampleTime", Tooltip: "Sample Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "QUERY_SAMPLE_TIMER_WAIT", IsPicoseconds: true},

	// MySQL 8.0.28+ CPU time
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpuTime", Tooltip: "CPU Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "SUM_CPU_TIME", IsPicoseconds: true, sortOpt: true, sortLbl: "Top queries by CPU Time"},

	// MySQL 8.0.31+ memory columns
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxControlledMemory", Tooltip: "Max Controlled Memory", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MAX_CONTROLLED_MEMORY", sortOpt: true, sortLbl: "Top queries by Max Controlled Memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTotalMemory", Tooltip: "Max Total Memory", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "MAX_TOTAL_MEMORY", sortOpt: true, sortLbl: "Top queries by Max Total Memory"},
}

type topQueriesChartGroup struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}

var topQueriesChartGroups = []topQueriesChartGroup{
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

var topQueriesLabelColumns = map[string]bool{
	"schema": true,
}

const topQueriesPrimaryLabel = "schema"

// topQueriesRowScanner interface for testing
type topQueriesRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// funcTopQueries implements funcapi.MethodHandler for MySQL top-queries.
// All function-related logic is encapsulated here, keeping Collector focused on metrics collection.
type funcTopQueries struct {
	router *router

	stmtSummaryCols   map[string]bool
	stmtSummaryColsMu sync.RWMutex
}

func newFuncTopQueries(r *router) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.cfg.topQueriesDisabled() {
		return nil, fmt.Errorf("top-queries function disabled in configuration")
	}
	if _, err := f.router.deps.DB(); err != nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case topQueriesMethodID:
		return f.methodParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.cfg.topQueriesDisabled() {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}
	if _, err := f.router.deps.DB(); err != nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	switch method {
	case topQueriesMethodID:
		queryCtx, cancel := context.WithTimeout(ctx, f.router.cfg.topQueriesTimeout())
		defer cancel()
		return f.collectData(queryCtx, params.Column(topQueriesParamSort))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) methodParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	available, err := isPerformanceSchemaEnabled(ctx, f.router.deps)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, fmt.Errorf("performance_schema is not enabled")
	}

	availableCols, err := f.detectStatementsColumns(ctx)
	if err != nil {
		return nil, err
	}
	cols := f.buildAvailableColumns(availableCols)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in events_statements_summary_by_digest")
	}

	sortParam := f.buildSortParam(cols)
	return []funcapi.ParamConfig{sortParam}, nil
}

// collectData queries performance_schema for top queries using dynamic columns
func (f *funcTopQueries) collectData(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	if resp := f.checkPerformanceSchema(ctx); resp != nil {
		return resp
	}

	cols, availableCols, resp := f.detectAndFilterColumns(ctx)
	if resp != nil {
		return resp
	}

	data, resp := f.executeTopQueriesSQL(ctx, cols, availableCols, sortColumn, f.router.cfg.topQueriesLimit())
	if resp != nil {
		return resp
	}

	data, cols = f.enrichWithErrorAttribution(ctx, data, cols)
	return f.buildTopQueriesResponse(data, cols)
}

func (f *funcTopQueries) checkPerformanceSchema(ctx context.Context) *funcapi.FunctionResponse {
	available, err := isPerformanceSchemaEnabled(ctx, f.router.deps)
	if err != nil {
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to check performance_schema availability: %v", err),
		}
	}
	if !available {
		return &funcapi.FunctionResponse{
			Status:  503,
			Message: "performance_schema is not enabled",
		}
	}
	return nil
}

func (f *funcTopQueries) detectAndFilterColumns(ctx context.Context) ([]topQueriesColumn, map[string]bool, *funcapi.FunctionResponse) {
	availableCols, err := f.detectStatementsColumns(ctx)
	if err != nil {
		return nil, nil, &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	cols := f.buildAvailableColumns(availableCols)
	if len(cols) == 0 {
		return nil, nil, &funcapi.FunctionResponse{
			Status:  500,
			Message: "no columns available in events_statements_summary_by_digest",
		}
	}
	return cols, availableCols, nil
}

func (f *funcTopQueries) executeTopQueriesSQL(ctx context.Context, cols []topQueriesColumn, availableCols map[string]bool, sortColumn string, limit int) ([][]any, *funcapi.FunctionResponse) {
	dbSortColumn := f.mapAndValidateSortColumn(sortColumn, availableCols)
	query := f.buildDynamicSQL(cols, dbSortColumn, limit)

	db, err := f.router.deps.DB()
	if err != nil {
		return nil, &funcapi.FunctionResponse{Status: 503, Message: "collector database is unavailable"}
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &funcapi.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return nil, &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	data, err := f.scanDynamicRows(rows, cols)
	if err != nil {
		return nil, &funcapi.FunctionResponse{Status: 500, Message: err.Error()}
	}
	return data, nil
}

func (f *funcTopQueries) enrichWithErrorAttribution(ctx context.Context, data [][]any, cols []topQueriesColumn) ([][]any, []topQueriesColumn) {
	errorCols := mysqlErrorAttributionColumns()
	errorStatus := mysqlErrorAttrNotSupported
	errorDetails := map[string]mysqlErrorRow{}

	digestIdx := -1
	for i, col := range cols {
		if col.Name == "digest" {
			digestIdx = i
			break
		}
	}

	if digestIdx >= 0 {
		digests := make([]string, 0, len(data))
		seen := make(map[string]bool)
		for _, row := range data {
			if digestIdx >= len(row) {
				continue
			}
			digest, ok := row[digestIdx].(string)
			if !ok || digest == "" || seen[digest] {
				continue
			}
			seen[digest] = true
			digests = append(digests, digest)
		}
		if len(digests) > 0 {
			errorStatus, errorDetails = collectMySQLErrorDetailsForDigests(ctx, f.router.deps, f.router.cfg, f.router.log, digests)
		} else {
			errorStatus = mysqlErrorAttrNoData
		}
	}

	if len(errorCols) == 0 {
		return data, cols
	}

	for i := range data {
		status := errorStatus
		var errRow mysqlErrorRow
		var errNo any
		if digestIdx >= 0 && digestIdx < len(data[i]) {
			if digest, ok := data[i][digestIdx].(string); ok && digest != "" {
				if row, ok := errorDetails[digest]; ok {
					status = mysqlErrorAttrEnabled
					errRow = row
					if errRow.ErrorNumber != nil {
						errNo = *errRow.ErrorNumber
					}
				} else if status == mysqlErrorAttrEnabled {
					status = mysqlErrorAttrNoData
				}
			}
		}

		data[i] = append(data[i],
			status,
			errNo,
			nullableString(errRow.SQLState),
			nullableString(errRow.Message),
		)
	}

	return data, append(cols, errorCols...)
}

func (f *funcTopQueries) buildTopQueriesResponse(data [][]any, cols []topQueriesColumn) *funcapi.FunctionResponse {
	sortParam := f.buildSortParam(cols)
	sortOptions := sortParam.Options

	defaultSort := ""
	for _, col := range cols {
		if col.IsDefaultSort() && col.IsSortOption() {
			defaultSort = col.Name
			break
		}
	}
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	annotatedCols := f.decorateColumns(cols)
	cs := f.columnSet(annotatedCols)

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from performance_schema.events_statements_summary_by_digest",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		ChartingConfig:    cs.BuildCharting(),
	}
}

func (f *funcTopQueries) detectStatementsColumns(ctx context.Context) (map[string]bool, error) {
	f.stmtSummaryColsMu.RLock()
	if f.stmtSummaryCols != nil {
		cols := f.stmtSummaryCols
		f.stmtSummaryColsMu.RUnlock()
		return cols, nil
	}
	f.stmtSummaryColsMu.RUnlock()

	f.stmtSummaryColsMu.Lock()
	defer f.stmtSummaryColsMu.Unlock()

	if f.stmtSummaryCols != nil {
		return f.stmtSummaryCols, nil
	}

	db, err := f.router.deps.DB()
	if err != nil {
		return nil, err
	}
	cols, err := sqlquery.FetchTableColumns(
		ctx,
		db,
		"performance_schema",
		"events_statements_summary_by_digest",
		sqlquery.PlaceholderQuestion,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query column information: %w", err)
	}

	f.stmtSummaryCols = cols
	return cols, nil
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

// buildAvailableColumns filters columns based on what's available in the database
func (f *funcTopQueries) buildAvailableColumns(availableCols map[string]bool) []topQueriesColumn {
	var cols []topQueriesColumn
	for _, col := range topQueriesColumns {
		if availableCols[col.DBColumn] {
			cols = append(cols, col)
		}
	}
	return cols
}

// mapAndValidateSortColumn validates sort key and returns the ID to use
func (f *funcTopQueries) mapAndValidateSortColumn(sortKey string, availableCols map[string]bool) string {
	// Find the column by ID or DBColumn
	for _, col := range topQueriesColumns {
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

// buildDynamicSQL builds the SQL query with only available columns
func (f *funcTopQueries) buildDynamicSQL(cols []topQueriesColumn, sortColumn string, limit int) string {
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

// scanDynamicRows scans rows dynamically based on column types
func (f *funcTopQueries) scanDynamicRows(rows topQueriesRowScanner, cols []topQueriesColumn) ([][]any, error) {
	specs := make([]sqlquery.ScanColumnSpec, len(cols))
	for i, col := range cols {
		specs[i] = topQueriesScanSpec(col)
	}

	data, err := sqlquery.ScanTypedRows(rows, specs)
	if err != nil {
		return nil, fmt.Errorf("row scan failed: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return data, nil
}

func topQueriesScanSpec(col topQueriesColumn) sqlquery.ScanColumnSpec {
	spec := sqlquery.ScanColumnSpec{}
	switch col.Type {
	case funcapi.FieldTypeString:
		spec.Type = sqlquery.ScanValueString
		if col.Name == "query" || col.Name == "sampleQuery" {
			spec.Transform = func(v any) any {
				s, _ := v.(string)
				return strmutil.TruncateText(s, topQueriesMaxTextLength)
			}
		}
	case funcapi.FieldTypeInteger:
		spec.Type = sqlquery.ScanValueInteger
	case funcapi.FieldTypeDuration:
		spec.Type = sqlquery.ScanValueFloat
	default:
		spec.Type = sqlquery.ScanValueDiscard
	}
	return spec
}

func (f *funcTopQueries) buildSortParam(cols []topQueriesColumn) funcapi.ParamConfig {
	return funcapi.BuildSortParam(cols)
}

func (f *funcTopQueries) decorateColumns(cols []topQueriesColumn) []topQueriesColumn {
	out := make([]topQueriesColumn, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.Name] = i
	}

	for i := range out {
		if topQueriesLabelColumns[out[i].Name] {
			out[i].GroupBy = &funcapi.GroupByOptions{
				IsDefault: out[i].Name == topQueriesPrimaryLabel,
			}
		}
	}

	for _, group := range topQueriesChartGroups {
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
