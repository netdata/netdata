// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const ybMaxQueryTextLength = 4096

var errYBSQLDSNNotSet = errors.New("SQL DSN is not set")

// ybColumn embeds funcapi.ColumnMeta and adds YugabyteDB-specific fields.
type ybColumn struct {
	funcapi.ColumnMeta
	SelectExpr    string // SQL expression for SELECT clause
	IsSortOption  bool   // whether this column appears as a sort option
	SortLabel     string // label for sort option dropdown
	IsDefaultSort bool   // default sort column
	IsJoinColumn  bool   // column comes from a JOIN, not pg_stat_statements
}

func ybColumnSet(cols []ybColumn) funcapi.ColumnSet[ybColumn] {
	return funcapi.Columns(cols, func(c ybColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var ybTopColumns = []ybColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryId", Tooltip: "Query ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.queryid::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.query"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, SelectExpr: "d.datname", IsJoinColumn: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, GroupBy: &funcapi.GroupByOptions{}}, SelectExpr: "u.usename", IsJoinColumn: true},

	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Number of Calls", IsDefault: true}}, SelectExpr: "s.calls", IsSortOption: true, SortLabel: "Top queries by Calls"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}}, SelectExpr: "s.total_time", IsSortOption: true, IsDefaultSort: true, SortLabel: "Top queries by Total Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "meanTime", Tooltip: "Mean Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.mean_time", IsSortOption: true, SortLabel: "Top queries by Mean Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.min_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.max_time", IsSortOption: true, SortLabel: "Top queries by Max Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rows", Tooltip: "Rows", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}}, SelectExpr: "s.rows", IsSortOption: true, SortLabel: "Top queries by Rows Returned"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stddevTime", Tooltip: "Stddev Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, SelectExpr: "s.stddev_time"},
}

var ybRunningColumns = []ybColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "pid", Tooltip: "PID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.pid::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}, SelectExpr: "s.query"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.datname"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.usename"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "state", Tooltip: "State", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.state"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "waitEventType", Tooltip: "Wait Event Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.wait_event_type"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "waitEvent", Tooltip: "Wait Event", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.wait_event"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "application", Tooltip: "Application", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.application_name"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientAddress", Tooltip: "Client Address", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, SelectExpr: "s.client_addr::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryStart", Tooltip: "Query Start", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "TO_CHAR(s.query_start, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "elapsedMs", Tooltip: "Elapsed", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "CASE WHEN s.query_start IS NULL THEN 0 ELSE EXTRACT(EPOCH FROM (clock_timestamp() - s.query_start)) * 1000 END", IsSortOption: true, IsDefaultSort: true, SortLabel: "Running queries by Elapsed Time"},
}

func yugabyteMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildYBSortParam(ybTopColumns),
			},
		},
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running SQL statements from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildYBSortParam(ybRunningColumns),
			},
		},
	}
}

// funcYugabyte implements funcapi.MethodHandler for YugabyteDB.
type funcYugabyte struct {
	collector *Collector
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcYugabyte)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcYugabyte) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		cols := ybTopColumns
		if f.collector.db != nil {
			if available, err := f.collector.availableTopColumns(ctx); err == nil {
				cols = available
			}
		}
		return []funcapi.ParamConfig{buildYBSortParam(cols)}, nil
	case "running-queries":
		return []funcapi.ParamConfig{buildYBSortParam(ybRunningColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcYugabyte) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if err := f.collector.ensureSQL(ctx); err != nil {
		status := 503
		if errors.Is(err, errYBSQLDSNNotSet) {
			status = 400
		}
		return funcapi.ErrorResponse(status, "%s", err.Error())
	}

	switch method {
	case "top-queries":
		return f.collector.collectTopQueries(ctx, params.Column("__sort"))
	case "running-queries":
		return f.collector.collectRunningQueries(ctx, params.Column("__sort"))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func yugabyteFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return &funcYugabyte{collector: c}
}

func (c *Collector) ensureSQL(ctx context.Context) error {
	if c.db != nil {
		return nil
	}
	if c.DSN == "" {
		return errYBSQLDSNNotSet
	}

	db, err := sql.Open("pgx", c.DSN)
	if err != nil {
		return fmt.Errorf("error opening SQL connection: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	timeout := c.sqlTimeout()
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return fmt.Errorf("error pinging SQL connection: %w", err)
	}

	c.db = db
	return nil
}

func (c *Collector) sqlTimeout() time.Duration {
	if c.SQLTimeout.Duration() > 0 {
		return c.SQLTimeout.Duration()
	}
	return time.Second
}

func (c *Collector) availableTopColumns(ctx context.Context) ([]ybColumn, error) {
	available, err := c.detectPgStatStatementsColumns(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect available columns: %v", err)
	}
	cols := c.buildAvailableColumns(available)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no queryable columns found in pg_stat_statements")
	}
	return cols, nil
}

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	ok, err := c.pgStatStatementsEnabled(ctx)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("failed to check pg_stat_statements: %v", err)}
	}
	if !ok {
		return &module.FunctionResponse{
			Status: 503,
			Message: "pg_stat_statements extension is not installed in this database. " +
				"Run 'CREATE EXTENSION pg_stat_statements;' in the database the collector connects to.",
		}
	}

	cols, err := c.availableTopColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	sortColumn = resolveYBSortColumn(cols, sortColumn)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	query := buildYBTopQueriesSQL(cols, sortColumn)
	queryCtx, cancel := context.WithTimeout(ctx, c.sqlTimeout())
	defer cancel()
	rows, err := c.db.QueryContext(queryCtx, query, limit)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	data, err := scanYBRows(rows, cols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := ybColumnSet(cols)
	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildYBSortParam(cols)},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (c *Collector) collectRunningQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	sortColumn = resolveYBSortColumn(ybRunningColumns, sortColumn)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	query := buildYBRunningQueriesSQL(sortColumn)
	queryCtx, cancel := context.WithTimeout(ctx, c.sqlTimeout())
	defer cancel()
	rows, err := c.db.QueryContext(queryCtx, query, limit)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	data, err := scanYBRows(rows, ybRunningColumns)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := ybColumnSet(ybRunningColumns)
	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildYBSortParam(ybRunningColumns)},
	}
}

func buildYBSortParam(cols []ybColumn) funcapi.ParamConfig {
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

func resolveYBSortColumn(cols []ybColumn, requested string) string {
	if requested != "" {
		for _, col := range cols {
			if col.Name == requested && col.IsSortOption {
				return col.Name
			}
		}
	}
	for _, col := range cols {
		if col.IsDefaultSort && col.IsSortOption {
			return col.Name
		}
	}
	for _, col := range cols {
		if col.IsSortOption {
			return col.Name
		}
	}
	if len(cols) > 0 {
		return cols[0].Name
	}
	return ""
}

func buildYBTopQueriesSQL(cols []ybColumn, sortColumn string) string {
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

func buildYBRunningQueriesSQL(sortColumn string) string {
	selectCols := make([]string, 0, len(ybRunningColumns))
	for _, col := range ybRunningColumns {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.SelectExpr, col.Name))
	}
	return fmt.Sprintf(`
SELECT %s
FROM pg_stat_activity s
WHERE s.state IS DISTINCT FROM 'idle'
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func scanYBRows(rows *sql.Rows, cols []ybColumn) ([][]any, error) {
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
						s = strmutil.TruncateText(s, ybMaxQueryTextLength)
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

func (c *Collector) pgStatStatementsEnabled(ctx context.Context) (bool, error) {
	query := `SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements'`
	queryCtx, cancel := context.WithTimeout(ctx, c.sqlTimeout())
	defer cancel()
	var exists int
	if err := c.db.QueryRowContext(queryCtx, query).Scan(&exists); err != nil {
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

func (c *Collector) detectPgStatStatementsColumns(ctx context.Context) (map[string]bool, error) {
	c.pgStatStatementsMu.RLock()
	if c.pgStatStatementsColumns != nil {
		cols := c.pgStatStatementsColumns
		c.pgStatStatementsMu.RUnlock()
		return cols, nil
	}
	c.pgStatStatementsMu.RUnlock()

	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	if c.pgStatStatementsColumns != nil {
		return c.pgStatStatementsColumns, nil
	}

	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'pg_stat_statements'
		AND table_schema = 'public'
	`
	queryCtx, cancel := context.WithTimeout(ctx, c.sqlTimeout())
	defer cancel()

	rows, err := c.db.QueryContext(queryCtx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, fmt.Errorf("failed to scan column name: %v", err)
		}
		cols[colName] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %v", err)
	}

	c.pgStatStatementsColumns = cols
	return cols, nil
}

func (c *Collector) buildAvailableColumns(availableCols map[string]bool) []ybColumn {
	result := make([]ybColumn, 0, len(ybTopColumns))
	for _, col := range ybTopColumns {
		if col.IsJoinColumn {
			result = append(result, col)
			continue
		}
		actual, ok := resolveYBColumn(col.SelectExpr, availableCols)
		if !ok {
			continue
		}
		colCopy := col
		colCopy.SelectExpr = actual
		result = append(result, colCopy)
	}
	return result
}

func resolveYBColumn(expr string, availableCols map[string]bool) (string, bool) {
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
