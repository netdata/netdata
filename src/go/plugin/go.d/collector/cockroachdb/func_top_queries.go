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

const topQueriesMaxTextLength = 4096

// topQueriesColumn embeds funcapi.ColumnMeta and adds CockroachDB-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta
	SelectExpr    string // SQL expression for SELECT clause
	IsSortOption  bool   // whether this column appears as a sort option
	SortLabel     string // label for sort option dropdown
	IsDefaultSort bool   // default sort column
}

var topQueriesColumns = []topQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "fingerprintId", Tooltip: "Fingerprint ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.fingerprint_id::STRING"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.metadata->>'query'"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, SelectExpr: "s.metadata->>'db'"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "application", Tooltip: "Application", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "s.app_name"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "statementType", Tooltip: "Statement Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "s.metadata->>'stmtTyp'"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "distributed", Tooltip: "Distributed", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.metadata->>'distsql'"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "fullScan", Tooltip: "Full Scan", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.metadata->>'fullScan'"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "implicitTxn", Tooltip: "Implicit Txn", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.metadata->>'implicitTxn'"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "vectorized", Tooltip: "Vectorized", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.metadata->>'vec'"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "executions", Tooltip: "Executions", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Executions", IsDefault: true}}, SelectExpr: "COALESCE((s.statistics->'statistics'->>'cnt')::INT8, 0)", IsSortOption: true, SortLabel: "Top queries by Executions"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}}, SelectExpr: "COALESCE((s.statistics->'statistics'->'svcLat'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0) * 1000", IsSortOption: true, IsDefaultSort: true, SortLabel: "Top queries by Total Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "meanTime", Tooltip: "Mean Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "COALESCE((s.statistics->'statistics'->'svcLat'->>'mean')::FLOAT8, 0) * 1000", IsSortOption: true, SortLabel: "Top queries by Mean Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "runTime", Tooltip: "Run Time", Type: funcapi.FieldTypeDuration, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "COALESCE((s.statistics->'statistics'->'runLat'->>'mean')::FLOAT8, 0) * 1000"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "planTime", Tooltip: "Plan Time", Type: funcapi.FieldTypeDuration, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "COALESCE((s.statistics->'statistics'->'planLat'->>'mean')::FLOAT8, 0) * 1000"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "parseTime", Tooltip: "Parse Time", Type: funcapi.FieldTypeDuration, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "COALESCE((s.statistics->'statistics'->'parseLat'->>'mean')::FLOAT8, 0) * 1000"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsRead", Tooltip: "Rows Read", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'rowsRead'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", IsSortOption: true, SortLabel: "Top queries by Rows Read"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsWritten", Tooltip: "Rows Written", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'rowsWritten'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", IsSortOption: true, SortLabel: "Top queries by Rows Written"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsReturned", Tooltip: "Rows Returned", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'numRows'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", IsSortOption: true, SortLabel: "Top queries by Rows Returned"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "bytesRead", Tooltip: "Bytes Read", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Bytes", Title: "Bytes"}}, SelectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'bytesRead'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", IsSortOption: true, SortLabel: "Top queries by Bytes Read"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxRetries", Tooltip: "Max Retries", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Chart: &funcapi.ChartOptions{Group: "Retries", Title: "Retries"}}, SelectExpr: "COALESCE((s.statistics->'statistics'->>'maxRetries')::INT8, 0)"},
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
	return []funcapi.ParamConfig{buildSortParam(topQueriesColumns)}, nil
}

func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
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
	queryCtx, cancel := context.WithTimeout(ctx, f.router.sqlTimeout())
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
		Help:              "Top SQL statements from crdb_internal.cluster_statement_statistics. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildSortParam(topQueriesColumns)},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (f *funcTopQueries) columnSet() funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(topQueriesColumns, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) resolveSortColumn(requested string) string {
	if requested != "" {
		for _, col := range topQueriesColumns {
			if col.Name == requested && col.IsSortOption {
				return col.Name
			}
		}
	}
	for _, col := range topQueriesColumns {
		if col.IsDefaultSort && col.IsSortOption {
			return col.Name
		}
	}
	for _, col := range topQueriesColumns {
		if col.IsSortOption {
			return col.Name
		}
	}
	if len(topQueriesColumns) > 0 {
		return topQueriesColumns[0].Name
	}
	return ""
}

func (f *funcTopQueries) buildSQL(sortColumn string) string {
	selectCols := make([]string, 0, len(topQueriesColumns))
	for _, col := range topQueriesColumns {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.SelectExpr, col.Name))
	}
	return fmt.Sprintf(`
SELECT %s
FROM crdb_internal.cluster_statement_statistics AS s
WHERE s.metadata->>'query' IS NOT NULL
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func (f *funcTopQueries) scanRows(rows *sql.Rows) ([][]any, error) {
	cols := topQueriesColumns
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
