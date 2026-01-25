// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const queriesMaxTextLength = 4096

const queriesParamSort = "__sort"

// queriesColumn defines a column for OracleDB queries functions.
// Embeds funcapi.ColumnMeta for UI display and adds collector-specific fields.
type queriesColumn struct {
	funcapi.ColumnMeta
	SelectExpr     string // SQL expression for SELECT clause
	IsSortOption   bool   // whether this column appears as a sort option
	SortLabel      string // label for sort option dropdown
	IsDefaultSort  bool   // default sort column
	RequiresColumn string // only include if this column exists in the view
}

func queriesColumnSet(cols []queriesColumn) funcapi.ColumnSet[queriesColumn] {
	return funcapi.Columns(cols, func(c queriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

type queriesTopLayout struct {
	cols []queriesColumn
	join string
}

var queriesTopColumns = []queriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlId", Tooltip: "SQL ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.sql_id"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.sql_text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "schema", Tooltip: "Schema", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, SelectExpr: "s.parsing_schema_name"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "executions", Tooltip: "Executions", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Executions", IsDefault: true}}, SelectExpr: "NVL(s.executions, 0)", IsSortOption: true, SortLabel: "Top queries by Executions"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}}, SelectExpr: "NVL(s.elapsed_time, 0) / 1000", IsSortOption: true, IsDefaultSort: true, SortLabel: "Top queries by Total Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "CASE WHEN NVL(s.executions,0) = 0 THEN 0 ELSE (s.elapsed_time / s.executions) / 1000 END", IsSortOption: true, SortLabel: "Top queries by Avg Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpuTime", Tooltip: "CPU Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "CPU", Title: "CPU Time"}}, SelectExpr: "NVL(s.cpu_time, 0) / 1000", IsSortOption: true, SortLabel: "Top queries by CPU Time"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "bufferGets", Tooltip: "Buffer Gets", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "IO", Title: "I/O"}}, SelectExpr: "NVL(s.buffer_gets, 0)", IsSortOption: true, SortLabel: "Top queries by Buffer Gets"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "diskReads", Tooltip: "Disk Reads", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "IO", Title: "I/O"}}, SelectExpr: "NVL(s.disk_reads, 0)", IsSortOption: true, SortLabel: "Top queries by Disk Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsProcessed", Tooltip: "Rows Processed", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "NVL(s.rows_processed, 0)", IsSortOption: true, SortLabel: "Top queries by Rows Processed"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "parseCalls", Tooltip: "Parse Calls", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Parse", Title: "Parse Calls"}}, SelectExpr: "NVL(s.parse_calls, 0)", IsSortOption: true, SortLabel: "Top queries by Parse Calls"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "module", Tooltip: "Module", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "s.module", RequiresColumn: "MODULE"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "action", Tooltip: "Action", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "s.action", RequiresColumn: "ACTION"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastActiveTime", Tooltip: "Last Active", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformText}, SelectExpr: "TO_CHAR(CAST(s.last_active_time AS TIMESTAMP), 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')", RequiresColumn: "LAST_ACTIVE_TIME"},
}

var queriesRunningColumns = []queriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "sessionId", Tooltip: "Session", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.sid || ',' || s.serial#"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "username", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.username"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "status", Tooltip: "Status", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.status"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "type", Tooltip: "Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.type"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlId", Tooltip: "SQL ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.sql_id"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "q.sql_text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastCallMs", Tooltip: "Elapsed", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "NVL(s.last_call_et, 0) * 1000", IsSortOption: true, IsDefaultSort: true, SortLabel: "Running queries by Elapsed Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlExecStart", Tooltip: "SQL Exec Start", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "TO_CHAR(CAST(s.sql_exec_start AS TIMESTAMP), 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "module", Tooltip: "Module", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.module"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "action", Tooltip: "Action", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.action"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "program", Tooltip: "Program", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.program"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "machine", Tooltip: "Machine", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.machine"},
}

// funcQueries implements funcapi.MethodHandler for OracleDB query functions.
// Handles both top-queries and running-queries methods.
type funcQueries struct {
	collector *Collector
}

func newFuncQueries(c *Collector) *funcQueries {
	return &funcQueries{collector: c}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcQueries)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		cols := queriesTopColumns
		if f.collector.db != nil {
			cols = f.topLayout(ctx).cols
		}
		return []funcapi.ParamConfig{buildQueriesSortParam(cols)}, nil
	case "running-queries":
		return []funcapi.ParamConfig{buildQueriesSortParam(queriesRunningColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.collector.db == nil {
		if err := f.collector.openConnection(); err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
	}

	switch method {
	case "top-queries":
		return f.collectTopQueries(ctx, params.Column(queriesParamSort))
	case "running-queries":
		return f.collectRunningQueries(ctx, params.Column(queriesParamSort))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func (f *funcQueries) topLayout(ctx context.Context) queriesTopLayout {
	available, err := f.fetchSQLStatsColumns(ctx)
	if err != nil {
		return queriesTopLayout{cols: filterQueriesColumns(queriesTopColumns, nil)}
	}
	return buildQueriesTopLayout(available)
}

func (f *funcQueries) fetchSQLStatsColumns(ctx context.Context) (map[string]bool, error) {
	rows, err := f.collector.db.QueryContext(ctx, "SELECT * FROM v$sqlstats WHERE 1=0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	cols := make(map[string]bool, len(names))
	for _, name := range names {
		cols[strings.ToUpper(name)] = true
	}

	return cols, nil
}

func (f *funcQueries) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	layout := f.topLayout(ctx)
	topCols := layout.cols
	limit := f.collector.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	sortColumn = mapQueriesSortColumn(sortColumn, topCols)
	if sortColumn == "" {
		return &module.FunctionResponse{Status: 500, Message: "no sortable columns available"}
	}

	joinClause := ""
	if layout.join != "" {
		joinClause = "\n" + layout.join
	}
	query := fmt.Sprintf(`
SELECT %s
FROM v$sqlstats s
%s
WHERE NVL(s.executions, 0) > 0
ORDER BY %s DESC NULLS LAST
FETCH FIRST %d ROWS ONLY
`, buildQueriesSelect(topCols), joinClause, sortColumn, limit)

	rows, err := f.collector.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("top queries query failed: %v", err)}
	}
	defer func() { _ = rows.Close() }()

	data, err := scanQueriesRows(rows, topCols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := queriesColumnSet(topCols)
	if len(data) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No SQL statements found.",
			Help:              "Top SQL statements from V$SQLSTATS",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{buildQueriesSortParam(topCols)},
			Charts:            cs.BuildCharts(),
			DefaultCharts:     cs.BuildDefaultCharts(),
			GroupBy:           cs.BuildGroupBy(),
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildQueriesSortParam(topCols)},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (f *funcQueries) collectRunningQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := f.collector.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	sortColumn = mapQueriesSortColumn(sortColumn, queriesRunningColumns)
	if sortColumn == "" {
		return &module.FunctionResponse{Status: 500, Message: "no sortable columns available"}
	}

	query := fmt.Sprintf(`
SELECT %s
FROM v$session s
LEFT JOIN v$sql q
  ON q.sql_id = s.sql_id AND q.child_number = s.sql_child_number
WHERE s.type = 'USER'
  AND s.status = 'ACTIVE'
  AND s.sql_id IS NOT NULL
ORDER BY %s DESC NULLS LAST
FETCH FIRST %d ROWS ONLY
`, buildQueriesSelect(queriesRunningColumns), sortColumn, limit)

	rows, err := f.collector.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("running queries query failed: %v", err)}
	}
	defer func() { _ = rows.Close() }()

	data, err := scanQueriesRows(rows, queriesRunningColumns)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := queriesColumnSet(queriesRunningColumns)
	if len(data) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running queries found.",
			Help:              "Currently running SQL statements from V$SESSION",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{buildQueriesSortParam(queriesRunningColumns)},
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildQueriesSortParam(queriesRunningColumns)},
	}
}

// oracledbMethods returns the method configurations for registration.
func oracledbMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildQueriesSortParam(queriesTopColumns),
			},
		},
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildQueriesSortParam(queriesRunningColumns),
			},
		},
	}
}

// oracledbFunctionHandler returns the MethodHandler for an OracleDB job.
func oracledbFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcQueries
}

// buildQueriesSortParam builds the sort parameter for method registration.
func buildQueriesSortParam(cols []queriesColumn) funcapi.ParamConfig {
	var options []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.IsSortOption {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.Name,
			Column: col.Name,
			Name:   col.SortLabel,
			Sort:   &sortDir,
		}
		if col.IsDefaultSort {
			opt.Default = true
		}
		options = append(options, opt)
	}
	return funcapi.ParamConfig{
		ID:         queriesParamSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    options,
		UniqueView: true,
	}
}

func buildQueriesSelect(cols []queriesColumn) string {
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		expr := col.SelectExpr
		if expr == "" {
			expr = col.Name
		}
		parts = append(parts, fmt.Sprintf("%s AS %s", expr, col.Name))
	}
	return strings.Join(parts, ", ")
}

func mapQueriesSortColumn(input string, cols []queriesColumn) string {
	for _, col := range cols {
		if col.IsSortOption && col.Name == input {
			return col.Name
		}
	}
	for _, col := range cols {
		if col.IsDefaultSort {
			return col.Name
		}
	}
	for _, col := range cols {
		if col.IsSortOption {
			return col.Name
		}
	}
	return ""
}

func filterQueriesColumns(cols []queriesColumn, available map[string]bool) []queriesColumn {
	filtered := make([]queriesColumn, 0, len(cols))
	for _, col := range cols {
		if col.RequiresColumn != "" {
			if available == nil {
				continue
			}
			if !available[strings.ToUpper(col.RequiresColumn)] {
				continue
			}
		}
		filtered = append(filtered, col)
	}
	return filtered
}

func buildQueriesTopLayout(available map[string]bool) queriesTopLayout {
	schemaExpr, schemaJoin := resolveQueriesSchemaExpr(available)
	filtered := make([]queriesColumn, 0, len(queriesTopColumns))
	for _, col := range queriesTopColumns {
		if col.Name == "schema" {
			if schemaExpr == "" {
				continue
			}
			col.SelectExpr = schemaExpr
		}
		if col.RequiresColumn != "" && !available[strings.ToUpper(col.RequiresColumn)] {
			continue
		}
		filtered = append(filtered, col)
	}
	return queriesTopLayout{cols: filtered, join: schemaJoin}
}

func resolveQueriesSchemaExpr(available map[string]bool) (string, string) {
	switch {
	case available["PARSING_SCHEMA_NAME"]:
		return "s.parsing_schema_name", ""
	case available["PARSING_SCHEMA_ID"]:
		return "COALESCE(u.username, TO_CHAR(s.parsing_schema_id))", "LEFT JOIN all_users u ON u.user_id = s.parsing_schema_id"
	case available["LAST_EXEC_USER_ID"]:
		return "COALESCE(u.username, TO_CHAR(s.last_exec_user_id))", "LEFT JOIN all_users u ON u.user_id = s.last_exec_user_id"
	default:
		return "", ""
	}
}

func scanQueriesRows(rows *sql.Rows, cols []queriesColumn) ([][]any, error) {
	data := make([][]any, 0, 500)

	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))

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

		row := make([]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case *sql.NullString:
				if v.Valid {
					s := v.String
					if col.Name == "query" {
						s = strmutil.TruncateText(s, queriesMaxTextLength)
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
