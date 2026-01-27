// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	runningQueriesMethodID      = "running-queries"
	runningQueriesMaxTextLength = 4096
)

func runningQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             runningQueriesMethodID,
		Name:           "Running Queries",
		UpdateEvery:    10,
		Help:           "Currently running SQL statements from SHOW CLUSTER STATEMENTS. WARNING: Query text may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(runningQueriesColumns)},
	}
}

// runningQueriesColumn embeds funcapi.ColumnMeta and adds CockroachDB-specific fields.
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
func (c runningQueriesColumn) SortColumn() string  { return "" }

var runningQueriesColumns = []runningQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryId", Tooltip: "Query ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.query_id::STRING"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.query"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.user_name"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "application", Tooltip: "Application", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.application_name"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientAddress", Tooltip: "Client Address", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.client_address"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "nodeId", Tooltip: "Node ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.node_id::STRING"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sessionId", Tooltip: "Session ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.session_id::STRING"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "phase", Tooltip: "Phase", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.phase"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "distributed", Tooltip: "Distributed", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.distributed::STRING"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "startTime", Tooltip: "Start Time", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "TO_CHAR(s.start, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "elapsedMs", Tooltip: "Elapsed", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "EXTRACT(EPOCH FROM (clock_timestamp() - s.start)) * 1000", sortOpt: true, defaultSort: true, sortLbl: "Running queries by Elapsed Time"},
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
	if f.router.collector.Functions.RunningQueries.Disabled {
		return nil, fmt.Errorf("running-queries function disabled in configuration")
	}
	return []funcapi.ParamConfig{funcapi.BuildSortParam(runningQueriesColumns)}, nil
}

func (f *funcRunningQueries) Cleanup(ctx context.Context) {}

func (f *funcRunningQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.Functions.RunningQueries.Disabled {
		return funcapi.UnavailableResponse("running-queries function has been disabled in configuration")
	}
	if err := f.router.ensureDB(ctx); err != nil {
		status := 503
		if errors.Is(err, errSQLDSNNotSet) {
			status = 400
		}
		return funcapi.ErrorResponse(status, "%s", err)
	}

	sortColumn := f.resolveSortColumn(params.Column("__sort"))
	limit := f.router.topQueriesLimit()

	query := f.buildSQL(sortColumn)
	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.runningQueriesTimeout())
	defer cancel()

	rows, err := f.router.db.QueryContext(queryCtx, query, limit)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("query failed: %v", err)
	}
	defer rows.Close()

	data, err := f.scanRows(rows)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	cs := f.columnSet()
	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from SHOW CLUSTER STATEMENTS. WARNING: Query text may contain unmasked literals (potential PII).",
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
	if requested != "" {
		for _, col := range runningQueriesColumns {
			if col.Name == requested && col.IsSortOption() {
				return col.Name
			}
		}
	}
	for _, col := range runningQueriesColumns {
		if col.IsDefaultSort() && col.IsSortOption() {
			return col.Name
		}
	}
	for _, col := range runningQueriesColumns {
		if col.IsSortOption() {
			return col.Name
		}
	}
	if len(runningQueriesColumns) > 0 {
		return runningQueriesColumns[0].Name
	}
	return ""
}

func (f *funcRunningQueries) buildSQL(sortColumn string) string {
	selectCols := make([]string, 0, len(runningQueriesColumns))
	for _, col := range runningQueriesColumns {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.SelectExpr, col.Name))
	}
	return fmt.Sprintf(`
SELECT %s
FROM [SHOW CLUSTER STATEMENTS] AS s
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
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
			case funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
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
