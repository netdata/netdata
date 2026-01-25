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

const runningQueriesMaxTextLength = 4096

// runningQueriesColumn embeds funcapi.ColumnMeta and adds OracleDB-specific fields.
type runningQueriesColumn struct {
	funcapi.ColumnMeta
	SelectExpr  string // SQL expression for SELECT clause
	sortOpt     bool   // whether this column appears as a sort option
	sortLbl     string // label for sort option dropdown
	defaultSort bool   // default sort column
}

// funcapi.SortableColumn interface implementation for runningQueriesColumn.
func (c runningQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c runningQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c runningQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c runningQueriesColumn) ColumnName() string  { return c.Name }

var runningQueriesColumns = []runningQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "sessionId", Tooltip: "Session", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.sid || ',' || s.serial#"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "username", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.username"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "status", Tooltip: "Status", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.status"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "type", Tooltip: "Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.type"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlId", Tooltip: "SQL ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.sql_id"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "q.sql_text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastCallMs", Tooltip: "Elapsed", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "NVL(s.last_call_et, 0) * 1000", sortOpt: true, defaultSort: true, sortLbl: "Running queries by Elapsed Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlExecStart", Tooltip: "SQL Exec Start", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "TO_CHAR(CAST(s.sql_exec_start AS TIMESTAMP), 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "module", Tooltip: "Module", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.module"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "action", Tooltip: "Action", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.action"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "program", Tooltip: "Program", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.program"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "machine", Tooltip: "Machine", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.machine"},
}

// funcRunningQueries handles the running-queries function.
type funcRunningQueries struct {
	router *funcRouter
}

func newFuncRunningQueries(r *funcRouter) *funcRunningQueries {
	return &funcRunningQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRunningQueries)(nil)

func (f *funcRunningQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	return []funcapi.ParamConfig{funcapi.BuildSortParam(runningQueriesColumns)}, nil
}

func (f *funcRunningQueries) Cleanup(ctx context.Context) {}

func (f *funcRunningQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.db == nil {
		if err := f.router.collector.openConnection(); err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
	}

	limit := f.router.topQueriesLimit()

	sortColumn := f.resolveSortColumn(params.Column("__sort"))
	if sortColumn == "" {
		return funcapi.InternalErrorResponse("no sortable columns available")
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
`, f.buildSelectClause(), sortColumn, limit)

	rows, err := f.router.collector.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("running queries query failed: %v", err)
	}
	defer rows.Close()

	data, err := f.scanRows(rows)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	cs := f.columnSet()
	if len(data) == 0 {
		return &funcapi.FunctionResponse{
			Status:            200,
			Message:           "No running queries found.",
			Help:              "Currently running SQL statements from V$SESSION",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{funcapi.BuildSortParam(runningQueriesColumns)},
		}
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{funcapi.BuildSortParam(runningQueriesColumns)},
	}
}

func (f *funcRunningQueries) columnSet() funcapi.ColumnSet[runningQueriesColumn] {
	return funcapi.Columns(runningQueriesColumns, func(c runningQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcRunningQueries) resolveSortColumn(requested string) string {
	for _, col := range runningQueriesColumns {
		if col.IsSortOption() && col.Name == requested {
			return col.Name
		}
	}
	for _, col := range runningQueriesColumns {
		if col.IsDefaultSort() {
			return col.Name
		}
	}
	for _, col := range runningQueriesColumns {
		if col.IsSortOption() {
			return col.Name
		}
	}
	return ""
}

func (f *funcRunningQueries) buildSelectClause() string {
	parts := make([]string, 0, len(runningQueriesColumns))
	for _, col := range runningQueriesColumns {
		expr := col.SelectExpr
		if expr == "" {
			expr = col.Name
		}
		parts = append(parts, fmt.Sprintf("%s AS %s", expr, col.Name))
	}
	return strings.Join(parts, ", ")
}

func (f *funcRunningQueries) scanRows(rows *sql.Rows) ([][]any, error) {
	cols := runningQueriesColumns
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
						s = strmutil.TruncateText(s, runningQueriesMaxTextLength)
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
