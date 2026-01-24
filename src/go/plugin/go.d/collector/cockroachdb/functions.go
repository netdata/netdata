// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

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

const crdbMaxQueryTextLength = 4096

var errSQLDSNNotSet = errors.New("SQL DSN is not set")

type crdbColumn struct {
	funcapi.ColumnMeta
	SelectExpr    string // SQL expression for SELECT clause
	IsSortOption  bool   // whether this column appears as a sort option
	SortLabel     string // label for sort option dropdown
	IsDefaultSort bool   // default sort column
}

func crdbColumnSet(cols []crdbColumn) funcapi.ColumnSet[crdbColumn] {
	return funcapi.Columns(cols, func(c crdbColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var crdbTopColumns = []crdbColumn{
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

var crdbRunningColumns = []crdbColumn{
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
	{ColumnMeta: funcapi.ColumnMeta{Name: "elapsedMs", Tooltip: "Elapsed", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, SelectExpr: "EXTRACT(EPOCH FROM (clock_timestamp() - s.start)) * 1000", IsSortOption: true, IsDefaultSort: true, SortLabel: "Running queries by Elapsed Time"},
}

// funcCockroach implements funcapi.MethodHandler for CockroachDB.
type funcCockroach struct {
	collector *Collector
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcCockroach)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcCockroach) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return []funcapi.ParamConfig{buildCrdbSortParam(crdbTopColumns)}, nil
	case "running-queries":
		return []funcapi.ParamConfig{buildCrdbSortParam(crdbRunningColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcCockroach) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if err := f.collector.ensureSQL(ctx); err != nil {
		status := 503
		if errors.Is(err, errSQLDSNNotSet) {
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

func cockroachMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL statements from crdb_internal.cluster_statement_statistics. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildCrdbSortParam(crdbTopColumns),
			},
		},
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running SQL statements from SHOW CLUSTER STATEMENTS. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildCrdbSortParam(crdbRunningColumns),
			},
		},
	}
}

func cockroachFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return &funcCockroach{collector: c}
}

func (c *Collector) ensureSQL(ctx context.Context) error {
	if c.db != nil {
		return nil
	}
	if c.DSN == "" {
		return errSQLDSNNotSet
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

	setCtx, cancel := context.WithTimeout(ctx, timeout)
	if _, err := db.ExecContext(setCtx, "SET allow_unsafe_internals = on"); err != nil {
		c.Debugf("unable to set allow_unsafe_internals: %v", err)
	}
	cancel()

	c.db = db
	return nil
}

func (c *Collector) sqlTimeout() time.Duration {
	if c.SQLTimeout.Duration() > 0 {
		return c.SQLTimeout.Duration()
	}
	return time.Second
}

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	sortColumn = resolveCrdbSortColumn(crdbTopColumns, sortColumn)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	query := buildCrdbTopQueriesSQL(sortColumn, limit)
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

	data, err := scanCrdbRows(rows, crdbTopColumns)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := crdbColumnSet(crdbTopColumns)
	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL statements from crdb_internal.cluster_statement_statistics. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildCrdbSortParam(crdbTopColumns)},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (c *Collector) collectRunningQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	sortColumn = resolveCrdbSortColumn(crdbRunningColumns, sortColumn)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	query := buildCrdbRunningQueriesSQL(sortColumn, limit)
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

	data, err := scanCrdbRows(rows, crdbRunningColumns)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	cs := crdbColumnSet(crdbRunningColumns)
	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from SHOW CLUSTER STATEMENTS. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildCrdbSortParam(crdbRunningColumns)},
	}
}

func buildCrdbSortParam(cols []crdbColumn) funcapi.ParamConfig {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.IsSortOption {
			continue
		}
		sortOptions = append(sortOptions, funcapi.ParamOption{
			ID:      col.Name,
			Column:  col.Name,
			Name:    col.SortLabel,
			Sort:    &sortDir,
			Default: col.IsDefaultSort,
		})
	}
	return funcapi.ParamConfig{
		ID:         "__sort",
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    sortOptions,
		UniqueView: true,
	}
}

func resolveCrdbSortColumn(cols []crdbColumn, requested string) string {
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
	return ""
}

func buildCrdbTopQueriesSQL(sortColumn string, limit int) string {
	selectCols := make([]string, 0, len(crdbTopColumns))
	for _, col := range crdbTopColumns {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.SelectExpr, col.Name))
	}
	return fmt.Sprintf(`
SELECT %s
FROM crdb_internal.cluster_statement_statistics AS s
WHERE s.metadata->>'query' IS NOT NULL
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func buildCrdbRunningQueriesSQL(sortColumn string, limit int) string {
	selectCols := make([]string, 0, len(crdbRunningColumns))
	for _, col := range crdbRunningColumns {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.SelectExpr, col.Name))
	}
	return fmt.Sprintf(`
SELECT %s
FROM [SHOW CLUSTER STATEMENTS] AS s
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func scanCrdbRows(rows *sql.Rows, cols []crdbColumn) ([][]any, error) {
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
						s = strmutil.TruncateText(s, crdbMaxQueryTextLength)
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
