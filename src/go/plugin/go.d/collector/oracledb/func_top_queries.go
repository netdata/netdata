// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	topQueriesMethodID      = "top-queries"
	topQueriesMaxTextLength = 4096
)

func topQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)},
	}
}

// topQueriesColumn embeds funcapi.ColumnMeta and adds OracleDB-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta
	SelectExpr     string // SQL expression for SELECT clause
	sortOpt        bool   // whether this column appears as a sort option
	sortLbl        string // label for sort option dropdown
	defaultSort    bool   // default sort column
	RequiresColumn string // only include if this column exists in the view
}

// funcapi.SortableColumn interface implementation for topQueriesColumn.
func (c topQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c topQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c topQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c topQueriesColumn) ColumnName() string  { return c.Name }
func (c topQueriesColumn) SortColumn() string  { return "" }

var topQueriesColumns = []topQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlId", Tooltip: "SQL ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.sql_id"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.sql_text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "schema", Tooltip: "Schema", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, SelectExpr: "s.parsing_schema_name"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "executions", Tooltip: "Executions", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Executions", IsDefault: true}}, SelectExpr: "NVL(s.executions, 0)", sortOpt: true, sortLbl: "Top queries by Executions"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}}, SelectExpr: "NVL(s.elapsed_time, 0) / 1000", sortOpt: true, defaultSort: true, sortLbl: "Top queries by Total Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "CASE WHEN NVL(s.executions,0) = 0 THEN 0 ELSE (s.elapsed_time / s.executions) / 1000 END", sortOpt: true, sortLbl: "Top queries by Avg Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpuTime", Tooltip: "CPU Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "CPU", Title: "CPU Time"}}, SelectExpr: "NVL(s.cpu_time, 0) / 1000", sortOpt: true, sortLbl: "Top queries by CPU Time"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "bufferGets", Tooltip: "Buffer Gets", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "IO", Title: "I/O"}}, SelectExpr: "NVL(s.buffer_gets, 0)", sortOpt: true, sortLbl: "Top queries by Buffer Gets"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "diskReads", Tooltip: "Disk Reads", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "IO", Title: "I/O"}}, SelectExpr: "NVL(s.disk_reads, 0)", sortOpt: true, sortLbl: "Top queries by Disk Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsProcessed", Tooltip: "Rows Processed", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "NVL(s.rows_processed, 0)", sortOpt: true, sortLbl: "Top queries by Rows Processed"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "parseCalls", Tooltip: "Parse Calls", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Parse", Title: "Parse Calls"}}, SelectExpr: "NVL(s.parse_calls, 0)", sortOpt: true, sortLbl: "Top queries by Parse Calls"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "module", Tooltip: "Module", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "s.module", RequiresColumn: "MODULE"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "action", Tooltip: "Action", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "s.action", RequiresColumn: "ACTION"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastActiveTime", Tooltip: "Last Active", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformText}, SelectExpr: "TO_CHAR(CAST(s.last_active_time AS TIMESTAMP), 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')", RequiresColumn: "LAST_ACTIVE_TIME"},
}

type topQueriesLayout struct {
	cols []topQueriesColumn
	join string
}

// funcTopQueries handles the top-queries function.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.collector.Functions.TopQueries.Disabled {
		return nil, fmt.Errorf("top-queries function disabled in configuration")
	}
	cols := topQueriesColumns
	if f.router.collector.db != nil {
		cols = f.layout(ctx).cols
	}
	return []funcapi.ParamConfig{funcapi.BuildSortParam(cols)}, nil
}

func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.Functions.TopQueries.Disabled {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}
	if f.router.collector.db == nil {
		if err := f.router.collector.openConnection(); err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
	}

	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
	defer cancel()

	layout := f.layout(queryCtx)
	cols := layout.cols
	limit := f.router.topQueriesLimit()

	sortColumn := f.resolveSortColumn(cols, params.Column("__sort"))
	if sortColumn == "" {
		return funcapi.InternalErrorResponse("no sortable columns available")
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
`, f.buildSelectClause(cols), joinClause, sortColumn, limit)

	rows, err := f.router.collector.db.QueryContext(queryCtx, query)
	if err != nil {
		if queryCtx.Err() == context.DeadlineExceeded {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("top queries query failed: %v", err)
	}
	defer rows.Close()

	data, err := f.scanRows(rows, cols)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	cs := f.columnSet(cols)
	if len(data) == 0 {
		return &funcapi.FunctionResponse{
			Status:            200,
			Message:           "No SQL statements found.",
			Help:              "Top SQL statements from V$SQLSTATS",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{funcapi.BuildSortParam(cols)},
			ChartingConfig:    cs.BuildCharting(),
		}
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{funcapi.BuildSortParam(cols)},
		ChartingConfig:    cs.BuildCharting(),
	}
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) layout(ctx context.Context) topQueriesLayout {
	available, err := f.fetchSQLStatsColumns(ctx)
	if err != nil {
		return topQueriesLayout{cols: f.filterColumns(topQueriesColumns, nil)}
	}
	return f.buildLayout(available)
}

func (f *funcTopQueries) fetchSQLStatsColumns(ctx context.Context) (map[string]bool, error) {
	rows, err := f.router.collector.db.QueryContext(ctx, "SELECT * FROM v$sqlstats WHERE 1=0")
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

func (f *funcTopQueries) buildLayout(available map[string]bool) topQueriesLayout {
	schemaExpr, schemaJoin := f.resolveSchemaExpr(available)
	filtered := make([]topQueriesColumn, 0, len(topQueriesColumns))
	for _, col := range topQueriesColumns {
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
	return topQueriesLayout{cols: filtered, join: schemaJoin}
}

func (f *funcTopQueries) resolveSchemaExpr(available map[string]bool) (string, string) {
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

func (f *funcTopQueries) filterColumns(cols []topQueriesColumn, available map[string]bool) []topQueriesColumn {
	filtered := make([]topQueriesColumn, 0, len(cols))
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

func (f *funcTopQueries) resolveSortColumn(cols []topQueriesColumn, requested string) string {
	for _, col := range cols {
		if col.IsSortOption() && col.Name == requested {
			return col.Name
		}
	}
	for _, col := range cols {
		if col.IsDefaultSort() {
			return col.Name
		}
	}
	for _, col := range cols {
		if col.IsSortOption() {
			return col.Name
		}
	}
	return ""
}

func (f *funcTopQueries) buildSelectClause(cols []topQueriesColumn) string {
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

func (f *funcTopQueries) scanRows(rows *sql.Rows, cols []topQueriesColumn) ([][]any, error) {
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
						s = strmutil.TruncateText(s, topQueriesMaxTextLength)
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
