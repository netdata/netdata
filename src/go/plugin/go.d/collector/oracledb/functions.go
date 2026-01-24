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

const oracleMaxQueryTextLength = 4096

// oracleColumn embeds funcapi.ColumnMeta and adds Oracle-specific fields.
type oracleColumn struct {
	funcapi.ColumnMeta
	SelectExpr     string // SQL expression for SELECT clause
	IsSortOption   bool   // whether this column appears as a sort option
	SortLabel      string // label for sort option dropdown
	IsDefaultSort  bool   // default sort column
	RequiresColumn string // only include if this column exists in the view
}

func oracleColumnSet(cols []oracleColumn) funcapi.ColumnSet[oracleColumn] {
	return funcapi.Columns(cols, func(c oracleColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

type oracleTopLayout struct {
	cols []oracleColumn
	join string
}

var oracleTopColumns = []oracleColumn{
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

var oracleRunningColumns = []oracleColumn{
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

func oracleMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildOracleSortParam(oracleTopColumns),
			},
		},
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildOracleSortParam(oracleRunningColumns),
			},
		},
	}
}

func oracleMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	switch method {
	case "top-queries":
		cols := oracleTopColumns
		if collector.db != nil {
			cols = collector.oracleTopLayout(ctx).cols
		}
		return []funcapi.ParamConfig{buildOracleSortParam(cols)}, nil
	case "running-queries":
		return []funcapi.ParamConfig{buildOracleSortParam(oracleRunningColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func oracleHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if collector.db == nil {
		if err := collector.openConnection(); err != nil {
			return &module.FunctionResponse{Status: 503, Message: "collector is still initializing, please retry in a few seconds"}
		}
	}

	switch method {
	case "top-queries":
		return collector.collectTopQueries(ctx, params.Column("__sort"))
	case "running-queries":
		return collector.collectRunningQueries(ctx, params.Column("__sort"))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

func buildOracleSortParam(cols []oracleColumn) funcapi.ParamConfig {
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
		ID:         "__sort",
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    options,
		UniqueView: true,
	}
}

func buildOracleSelect(cols []oracleColumn) string {
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

func mapOracleSortColumn(input string, cols []oracleColumn) string {
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

func (c *Collector) oracleTopLayout(ctx context.Context) oracleTopLayout {
	available, err := c.fetchSQLStatsColumns(ctx)
	if err != nil {
		return oracleTopLayout{cols: filterOracleColumns(oracleTopColumns, nil)}
	}
	return buildOracleTopLayout(available)
}

func filterOracleColumns(cols []oracleColumn, available map[string]bool) []oracleColumn {
	filtered := make([]oracleColumn, 0, len(cols))
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

func buildOracleTopLayout(available map[string]bool) oracleTopLayout {
	schemaExpr, schemaJoin := resolveOracleSchemaExpr(available)
	filtered := make([]oracleColumn, 0, len(oracleTopColumns))
	for _, col := range oracleTopColumns {
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
	return oracleTopLayout{cols: filtered, join: schemaJoin}
}

func resolveOracleSchemaExpr(available map[string]bool) (string, string) {
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

func (c *Collector) fetchSQLStatsColumns(ctx context.Context) (map[string]bool, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT * FROM v$sqlstats WHERE 1=0")
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

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	layout := c.oracleTopLayout(ctx)
	topCols := layout.cols
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	sortColumn = mapOracleSortColumn(sortColumn, topCols)
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
`, buildOracleSelect(topCols), joinClause, sortColumn, limit)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("top queries query failed: %v", err)}
	}
	defer func() { _ = rows.Close() }()

	data, err := scanOracleRows(rows, topCols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := oracleColumnSet(topCols)
	if len(data) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No SQL statements found.",
			Help:              "Top SQL statements from V$SQLSTATS",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(topCols)},
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
		RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(topCols)},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (c *Collector) collectRunningQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	sortColumn = mapOracleSortColumn(sortColumn, oracleRunningColumns)
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
`, buildOracleSelect(oracleRunningColumns), sortColumn, limit)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("running queries query failed: %v", err)}
	}
	defer func() { _ = rows.Close() }()

	data, err := scanOracleRows(rows, oracleRunningColumns)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := oracleColumnSet(oracleRunningColumns)
	if len(data) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running queries found.",
			Help:              "Currently running SQL statements from V$SESSION",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(oracleRunningColumns)},
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(oracleRunningColumns)},
	}
}

func scanOracleRows(rows *sql.Rows, cols []oracleColumn) ([][]any, error) {
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
						s = strmutil.TruncateText(s, oracleMaxQueryTextLength)
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

