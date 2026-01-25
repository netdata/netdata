// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const topQueriesMaxTextLength = 4096

const topQueriesParamSort = "__sort"

// topQueriesColumn defines a column for ClickHouse top-queries function.
// Embeds funcapi.ColumnMeta for UI display and adds collector-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta

	// Data access
	DBColumn   string // Column name in system.query_log (empty = computed)
	SelectExpr string // SQL expression for SELECT

	// Sort parameter metadata
	IsSortOption  bool   // Include in __sort parameter options
	SortLabel     string // Display label for sort option
	IsDefaultSort bool   // Default sort option
}

var topQueriesColumns = []topQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryId", Tooltip: "Query ID", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true, Sortable: true}, DBColumn: "normalized_query_hash", SelectExpr: "toString(normalized_query_hash)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true, Sortable: true}, DBColumn: "query", SelectExpr: "any(query)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, GroupBy: &funcapi.GroupByOptions{IsDefault: true}, Sortable: true}, DBColumn: "current_database", SelectExpr: "any(current_database)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, GroupBy: &funcapi.GroupByOptions{}, Sortable: true}, DBColumn: "user", SelectExpr: "any(user)"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Number of Calls", IsDefault: true}, Sortable: true}, SelectExpr: "count()", IsSortOption: true, SortLabel: "Number of Calls"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}, Sortable: true}, DBColumn: "query_duration_ms", SelectExpr: "sum(query_duration_ms)", IsSortOption: true, SortLabel: "Total Execution Time", IsDefaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}, Sortable: true}, DBColumn: "query_duration_ms", SelectExpr: "avg(query_duration_ms)", IsSortOption: true, SortLabel: "Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}, Sortable: true}, DBColumn: "query_duration_ms", SelectExpr: "min(query_duration_ms)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}, Sortable: true}, DBColumn: "query_duration_ms", SelectExpr: "max(query_duration_ms)"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "readRows", Tooltip: "Read Rows", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}, Sortable: true}, DBColumn: "read_rows", SelectExpr: "sum(read_rows)", IsSortOption: true, SortLabel: "Rows Read"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "readBytes", Tooltip: "Read Bytes", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Bytes", Title: "Bytes"}, Sortable: true}, DBColumn: "read_bytes", SelectExpr: "sum(read_bytes)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "writtenRows", Tooltip: "Written Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}, Sortable: true}, DBColumn: "written_rows", SelectExpr: "sum(written_rows)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "writtenBytes", Tooltip: "Written Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Bytes", Title: "Bytes"}, Sortable: true}, DBColumn: "written_bytes", SelectExpr: "sum(written_bytes)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "resultRows", Tooltip: "Result Rows", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}, Sortable: true}, DBColumn: "result_rows", SelectExpr: "sum(result_rows)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "resultBytes", Tooltip: "Result Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Bytes", Title: "Bytes"}, Sortable: true}, DBColumn: "result_bytes", SelectExpr: "sum(result_bytes)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "memoryUsage", Tooltip: "Max Memory", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 0, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Memory", Title: "Memory"}, Sortable: true}, DBColumn: "memory_usage", SelectExpr: "max(memory_usage)"},
}

type topQueriesJSONResponse struct {
	Data []map[string]any `json:"data"`
}

// funcTopQueries implements funcapi.MethodHandler for ClickHouse top-queries.
// All function-related logic is encapsulated here, keeping Collector focused on metrics collection.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.collector.httpClient == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case "top-queries":
		return f.methodParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.httpClient == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	switch method {
	case "top-queries":
		return f.collectData(ctx, params.Column(topQueriesParamSort))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) methodParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	available, err := f.detectQueryLogColumns(ctx)
	if err != nil {
		return nil, err
	}
	cols := f.buildAvailableColumns(available)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in system.query_log")
	}
	sortParam := funcapi.ParamConfig{
		ID:         topQueriesParamSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildSortOptions(cols),
		UniqueView: true,
	}
	return []funcapi.ParamConfig{sortParam}, nil
}

func (f *funcTopQueries) detectQueryLogColumns(ctx context.Context) (map[string]bool, error) {
	query := `
SELECT name
FROM system.columns
WHERE database = 'system' AND table = 'query_log'
FORMAT JSON`

	req, err := web.NewHTTPRequest(f.router.collector.RequestConfig)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.URL.RawQuery = makeURLQuery(query)

	var resp topQueriesJSONResponse
	if err := web.DoHTTP(f.router.collector.httpClient).RequestJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to query system.columns: %w", err)
	}

	cols := make(map[string]bool, len(resp.Data))
	for _, row := range resp.Data {
		if name, ok := row["name"].(string); ok {
			cols[name] = true
		}
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("system.query_log not available")
	}
	return cols, nil
}

func (f *funcTopQueries) collectData(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	availableCols, err := f.detectQueryLogColumns(ctx)
	if err != nil {
		return funcapi.ErrorResponse(503, "system.query_log not available: %v", err)
	}

	cols := f.buildAvailableColumns(availableCols)
	if len(cols) == 0 {
		return funcapi.ErrorResponse(500, "no columns available in system.query_log")
	}

	cs := f.columnSet(cols)
	sortColumn = f.mapAndValidateSortColumn(sortColumn, cs)

	limit := f.router.collector.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	groupKey := "normalized_query_hash"
	if !availableCols[groupKey] {
		groupKey = "query"
	}

	selectParts := make([]string, 0, len(cols))
	for _, col := range cols {
		selectParts = append(selectParts, fmt.Sprintf("%s AS `%s`", col.SelectExpr, col.Name))
	}

	query := fmt.Sprintf(`
SELECT %s
FROM system.query_log
WHERE type = 'QueryFinish'
GROUP BY %s
ORDER BY `+"`%s`"+` DESC
LIMIT %d
FORMAT JSON
`, strings.Join(selectParts, ", "), groupKey, sortColumn, limit)

	req, err := web.NewHTTPRequest(f.router.collector.RequestConfig)
	if err != nil {
		return funcapi.ErrorResponse(500, "%v", err)
	}
	req = req.WithContext(ctx)
	req.URL.RawQuery = makeURLQuery(query)

	var resp topQueriesJSONResponse
	if err := web.DoHTTP(f.router.collector.httpClient).RequestJSON(req, &resp); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.ErrorResponse(500, "query failed: %v", err)
	}

	data := make([][]any, 0, len(resp.Data))
	for _, rowMap := range resp.Data {
		row := make([]any, len(cols))
		for i, col := range cols {
			row[i] = f.normalizeValue(col, rowMap[col.Name])
		}
		data = append(data, row)
	}

	sortParam := funcapi.ParamConfig{
		ID:         topQueriesParamSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildSortOptions(cols),
		UniqueView: true,
	}

	defaultSort := "totalTime"
	if !cs.ContainsColumn(defaultSort) {
		defaultSort = "calls"
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from ClickHouse system.query_log",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) buildAvailableColumns(available map[string]bool) []topQueriesColumn {
	var cols []topQueriesColumn
	for _, col := range topQueriesColumns {
		if col.DBColumn == "" || available[col.DBColumn] {
			cols = append(cols, col)
		}
	}
	return cols
}

func (f *funcTopQueries) mapAndValidateSortColumn(input string, cs funcapi.ColumnSet[topQueriesColumn]) string {
	if cs.ContainsColumn(input) {
		return input
	}
	if cs.ContainsColumn("totalTime") {
		return "totalTime"
	}
	if cs.ContainsColumn("calls") {
		return "calls"
	}
	names := cs.Names()
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func (f *funcTopQueries) normalizeValue(col topQueriesColumn, v any) any {
	switch col.Type {
	case funcapi.FieldTypeInteger:
		switch val := v.(type) {
		case float64:
			return int64(val)
		case json.Number:
			if i, err := val.Int64(); err == nil {
				return i
			}
		case string:
			if i, err := strconv.ParseInt(val, 10, 64); err == nil {
				return i
			}
		}
		return int64(0)
	case funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
		switch val := v.(type) {
		case float64:
			return val
		case json.Number:
			if f, err := val.Float64(); err == nil {
				return f
			}
		case string:
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return f
			}
		}
		return float64(0)
	default:
		if s, ok := v.(string); ok {
			if col.Name == "query" {
				return strmutil.TruncateText(s, topQueriesMaxTextLength)
			}
			return s
		}
		if v == nil {
			return ""
		}
		if col.Name == "query" {
			return strmutil.TruncateText(fmt.Sprint(v), topQueriesMaxTextLength)
		}
		return fmt.Sprint(v)
	}
}
