// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
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
		Help:           "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)},
	}
}

// topQueriesColumn embeds funcapi.ColumnMeta and adds YugabyteDB-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta
	SelectExpr   string // SQL expression for SELECT clause
	sortOpt      bool   // whether this column appears as a sort option
	sortLbl      string // label for sort option dropdown
	defaultSort  bool   // default sort column
	IsJoinColumn bool   // column comes from a JOIN, not pg_stat_statements
}

var topQueriesColumns = []topQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryId", Tooltip: "Query ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.queryid::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.query"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, SelectExpr: "d.datname", IsJoinColumn: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "u.usename", IsJoinColumn: true},

	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Number of Calls", IsDefault: true}}, SelectExpr: "s.calls", sortOpt: true, sortLbl: "Top queries by Calls"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}}, SelectExpr: "s.total_time", sortOpt: true, defaultSort: true, sortLbl: "Top queries by Total Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "meanTime", Tooltip: "Mean Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.mean_time", sortOpt: true, sortLbl: "Top queries by Mean Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.min_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.max_time", sortOpt: true, sortLbl: "Top queries by Max Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rows", Tooltip: "Rows", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "s.rows", sortOpt: true, sortLbl: "Top queries by Rows Returned"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stddevTime", Tooltip: "Stddev Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.stddev_time"},
}

// funcapi.SortableColumn interface implementation for topQueriesColumn.
func (c topQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c topQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c topQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c topQueriesColumn) ColumnName() string  { return c.Name }
func (c topQueriesColumn) SortColumn() string  { return "" }

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
	if err := f.router.ensureDB(ctx); err != nil {
		return nil, nil // Use static RequiredParams
	}
	cols, err := f.availableColumns(ctx)
	if err != nil {
		return nil, nil // Use static RequiredParams
	}
	return []funcapi.ParamConfig{funcapi.BuildSortParam(cols)}, nil
}

func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.Functions.TopQueries.Disabled {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}
	if err := f.router.ensureDB(ctx); err != nil {
		status := 503
		if errors.Is(err, errSQLDSNNotSet) {
			status = 400
		}
		return funcapi.ErrorResponse(status, "%s", err)
	}

	ok, err := f.pgStatStatementsEnabled(ctx)
	if err != nil {
		return funcapi.InternalErrorResponse("failed to check pg_stat_statements: %v", err)
	}
	if !ok {
		return funcapi.UnavailableResponse(
			"pg_stat_statements extension is not installed in this database. " +
				"Run 'CREATE EXTENSION pg_stat_statements;' in the database the collector connects to.",
		)
	}

	cols, err := f.availableColumns(ctx)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	sortColumn := f.resolveSortColumn(cols, params.Column("__sort"))
	limit := f.router.topQueriesLimit()

	query := f.buildSQL(cols, sortColumn)
	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
	defer cancel()

	rows, err := f.router.db.QueryContext(queryCtx, query, limit)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("query failed: %v", err)
	}
	defer rows.Close()

	data, err := f.scanRows(rows, cols)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	cs := f.columnSet(cols)
	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
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

func (f *funcTopQueries) availableColumns(ctx context.Context) ([]topQueriesColumn, error) {
	available, err := f.detectPgStatStatementsColumns(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect available columns: %v", err)
	}
	cols := f.buildAvailableColumns(available)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no queryable columns found in pg_stat_statements")
	}
	return cols, nil
}

func (f *funcTopQueries) pgStatStatementsEnabled(ctx context.Context) (bool, error) {
	query := `SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'`
	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
	defer cancel()

	var exists int
	if err := f.router.db.QueryRowContext(queryCtx, query).Scan(&exists); err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return false, queryCtx.Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (f *funcTopQueries) detectPgStatStatementsColumns(ctx context.Context) (map[string]bool, error) {
	f.router.pgStatStatementsColumnsMu.RLock()
	if f.router.pgStatStatementsColumns != nil {
		cols := f.router.pgStatStatementsColumns
		f.router.pgStatStatementsColumnsMu.RUnlock()
		return cols, nil
	}
	f.router.pgStatStatementsColumnsMu.RUnlock()

	f.router.pgStatStatementsColumnsMu.Lock()
	defer f.router.pgStatStatementsColumnsMu.Unlock()

	if f.router.pgStatStatementsColumns != nil {
		return f.router.pgStatStatementsColumns, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
	defer cancel()

	cols, err := sqlquery.FetchTableColumns(
		queryCtx,
		f.router.db,
		"public",
		"pg_stat_statements",
		sqlquery.PlaceholderDollar,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}

	f.router.pgStatStatementsColumns = cols
	return cols, nil
}

func (f *funcTopQueries) buildAvailableColumns(availableCols map[string]bool) []topQueriesColumn {
	result := make([]topQueriesColumn, 0, len(topQueriesColumns))
	for _, col := range topQueriesColumns {
		if col.IsJoinColumn {
			result = append(result, col)
			continue
		}
		actual, ok := f.resolveColumnExpr(col.SelectExpr, availableCols)
		if !ok {
			continue
		}
		colCopy := col
		colCopy.SelectExpr = actual
		result = append(result, colCopy)
	}
	return result
}

func (f *funcTopQueries) resolveColumnExpr(expr string, availableCols map[string]bool) (string, bool) {
	colName := expr
	if idx := strings.LastIndex(colName, "."); idx != -1 {
		colName = colName[idx+1:]
	}

	castSuffix := ""
	if idx := strings.Index(colName, "::"); idx != -1 {
		castSuffix = colName[idx:]
		colName = colName[:idx]
	}

	actual := colName
	switch colName {
	case "total_time":
		if availableCols["total_exec_time"] {
			actual = "total_exec_time"
		}
	case "mean_time":
		if availableCols["mean_exec_time"] {
			actual = "mean_exec_time"
		}
	case "min_time":
		if availableCols["min_exec_time"] {
			actual = "min_exec_time"
		}
	case "max_time":
		if availableCols["max_exec_time"] {
			actual = "max_exec_time"
		}
	case "stddev_time":
		if availableCols["stddev_exec_time"] {
			actual = "stddev_exec_time"
		}
	}

	if !availableCols[actual] {
		return "", false
	}

	if actual == colName {
		return expr, true
	}
	return strings.Replace(expr, colName+castSuffix, actual+castSuffix, 1), true
}

func (f *funcTopQueries) resolveSortColumn(cols []topQueriesColumn, requested string) string {
	if requested != "" {
		for _, col := range cols {
			if col.Name == requested && col.IsSortOption() {
				return col.Name
			}
		}
	}
	for _, col := range cols {
		if col.IsDefaultSort() && col.IsSortOption() {
			return col.Name
		}
	}
	for _, col := range cols {
		if col.IsSortOption() {
			return col.Name
		}
	}
	if len(cols) > 0 {
		return cols[0].Name
	}
	return ""
}

func (f *funcTopQueries) buildSQL(cols []topQueriesColumn, sortColumn string) string {
	selectCols := make([]string, 0, len(cols))
	for _, col := range cols {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.SelectExpr, col.Name))
	}

	return fmt.Sprintf(`
SELECT %s
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_user u ON u.usesysid = s.userid
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func (f *funcTopQueries) scanRows(rows *sql.Rows, cols []topQueriesColumn) ([][]any, error) {
	specs := make([]sqlquery.ScanColumnSpec, len(cols))
	for i, col := range cols {
		specs[i] = yugabyteTopQueriesScanSpec(col)
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

func yugabyteTopQueriesScanSpec(col topQueriesColumn) sqlquery.ScanColumnSpec {
	spec := sqlquery.ScanColumnSpec{}
	switch col.Type {
	case funcapi.FieldTypeString:
		spec.Type = sqlquery.ScanValueString
		if col.Name == "query" {
			spec.Transform = func(v any) any {
				s, _ := v.(string)
				return strmutil.TruncateText(s, topQueriesMaxTextLength)
			}
		}
	case funcapi.FieldTypeInteger:
		spec.Type = sqlquery.ScanValueInteger
	case funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
		spec.Type = sqlquery.ScanValueFloat
	default:
		spec.Type = sqlquery.ScanValueDiscard
	}
	return spec
}
