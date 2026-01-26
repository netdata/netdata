// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const proxysqlMaxQueryTextLength = 4096

const (
	topQueriesMethodID = "top-queries"
	paramSort          = "__sort"
)

func topQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Top SQL queries from ProxySQL query digest stats",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(proxysqlAllColumns)},
	}
}

// proxysqlColumn defines metadata for a ProxySQL query digest column.
// Embeds funcapi.ColumnMeta for UI rendering and adds ProxySQL-specific fields.
type proxysqlColumn struct {
	funcapi.ColumnMeta

	// DBColumn is the database column name
	DBColumn string
	// IsMicroseconds indicates if the value is in microseconds (needs /1000 conversion)
	IsMicroseconds bool
	// sortOpt indicates whether this column appears in the sort dropdown
	sortOpt bool
	// sortLbl is the label shown in the sort dropdown
	sortLbl string
	// defaultSort indicates whether this is the default sort column
	defaultSort bool
}

// funcapi.SortableColumn interface implementation for proxysqlColumn.
func (c proxysqlColumn) IsSortOption() bool  { return c.sortOpt }
func (c proxysqlColumn) SortLabel() string   { return c.sortLbl }
func (c proxysqlColumn) IsDefaultSort() bool { return c.defaultSort }
func (c proxysqlColumn) ColumnName() string  { return c.Name }
func (c proxysqlColumn) SortColumn() string  { return "" }

// proxysqlColumnSet creates a ColumnSet from a slice of proxysqlColumn.
func proxysqlColumnSet(cols []proxysqlColumn) funcapi.ColumnSet[proxysqlColumn] {
	return funcapi.Columns(cols, func(c proxysqlColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var proxysqlAllColumns = []proxysqlColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "digest", Tooltip: "Digest", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true, Sortable: true}, DBColumn: "digest"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true, Sortable: true}, DBColumn: "digest_text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "schema", Tooltip: "Schema", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, GroupBy: &funcapi.GroupByOptions{IsDefault: true}, Sortable: true}, DBColumn: "schemaname"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, GroupBy: &funcapi.GroupByOptions{}, Sortable: true}, DBColumn: "username"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "hostgroup", Tooltip: "Hostgroup", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterRange, GroupBy: &funcapi.GroupByOptions{}, Sortable: true}, DBColumn: "hostgroup"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Calls", Title: "Number of Calls", IsDefault: true}, Sortable: true}, DBColumn: "count_star", sortOpt: true, sortLbl: "Top queries by Number of Calls"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}, Sortable: true}, DBColumn: "sum_time", IsMicroseconds: true, sortOpt: true, sortLbl: "Top queries by Total Execution Time", defaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}, Sortable: true}, DBColumn: "avg_time", IsMicroseconds: true, sortOpt: true, sortLbl: "Top queries by Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}, Sortable: true}, DBColumn: "min_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}, Sortable: true}, DBColumn: "max_time", IsMicroseconds: true},

	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsAffected", Tooltip: "Rows Affected", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}, Sortable: true}, DBColumn: "sum_rows_affected", sortOpt: true, sortLbl: "Top queries by Rows Affected"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "rowsSent", Tooltip: "Rows Sent", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Rows", Title: "Rows"}, Sortable: true}, DBColumn: "sum_rows_sent", sortOpt: true, sortLbl: "Top queries by Rows Sent"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "errors", Tooltip: "Errors", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Errors", Title: "Errors & Warnings"}, Sortable: true}, DBColumn: "sum_errors", sortOpt: true, sortLbl: "Top queries by Errors"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "warnings", Tooltip: "Warnings", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Chart: &funcapi.ChartOptions{Group: "Errors", Title: "Errors & Warnings"}, Sortable: true}, DBColumn: "sum_warnings", sortOpt: true, sortLbl: "Top queries by Warnings"},

	{ColumnMeta: funcapi.ColumnMeta{Name: "firstSeen", Tooltip: "First Seen", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "first_seen"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastSeen", Tooltip: "Last Seen", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "last_seen"},
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// funcTopQueries handles the "top-queries" function for ProxySQL.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topQueriesMethodID {
		return nil, nil
	}

	c := f.router.collector
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, err
		}
	}

	return c.topQueriesParams(ctx)
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topQueriesMethodID {
		return funcapi.NotFoundResponse(method)
	}

	c := f.router.collector
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return funcapi.UnavailableResponse(fmt.Sprintf("failed to open connection: %v", err))
		}
	}

	return c.collectTopQueries(ctx, params.Column(paramSort))
}

func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func buildProxySQLSortParam(cols []proxysqlColumn) funcapi.ParamConfig {
	return funcapi.BuildSortParam(cols)
}

func (c *Collector) detectProxySQLDigestColumns(ctx context.Context) (map[string]bool, error) {
	c.queryDigestColsMu.RLock()
	if c.queryDigestCols != nil {
		cols := c.queryDigestCols
		c.queryDigestColsMu.RUnlock()
		return cols, nil
	}
	c.queryDigestColsMu.RUnlock()

	c.queryDigestColsMu.Lock()
	defer c.queryDigestColsMu.Unlock()
	if c.queryDigestCols != nil {
		return c.queryDigestCols, nil
	}

	rows, err := c.db.QueryContext(ctx, "SELECT * FROM stats_mysql_query_digest WHERE 1=0")
	if err != nil {
		return nil, fmt.Errorf("failed to query stats_mysql_query_digest columns: %w", err)
	}
	defer rows.Close()

	names, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to read columns: %w", err)
	}

	cols := make(map[string]bool, len(names))
	for _, name := range names {
		cols[strings.ToLower(name)] = true
	}
	c.queryDigestCols = cols
	return cols, nil
}

func (c *Collector) buildAvailableProxySQLColumns(available map[string]bool) []proxysqlColumn {
	var cols []proxysqlColumn
	for _, col := range proxysqlAllColumns {
		if col.DBColumn == "" || available[strings.ToLower(col.DBColumn)] {
			cols = append(cols, col)
		}
	}
	return cols
}

func (c *Collector) topQueriesParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	availableCols, err := c.detectProxySQLDigestColumns(ctx)
	if err != nil {
		return nil, err
	}
	cols := c.buildAvailableProxySQLColumns(availableCols)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in stats_mysql_query_digest")
	}
	return []funcapi.ParamConfig{buildProxySQLSortParam(cols)}, nil
}

func (c *Collector) mapAndValidateProxySQLSortColumn(input string, cs funcapi.ColumnSet[proxysqlColumn]) string {
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

func (c *Collector) buildProxySQLDynamicSQL(cols []proxysqlColumn, sortColumn string, limit int) string {
	selectParts := make([]string, 0, len(cols))
	for _, col := range cols {
		expr := col.DBColumn
		if col.IsMicroseconds {
			expr = fmt.Sprintf("%s/1000", col.DBColumn)
		}
		selectParts = append(selectParts, fmt.Sprintf("%s AS `%s`", expr, col.Name))
	}

	return fmt.Sprintf(`
SELECT %s
FROM stats_mysql_query_digest
ORDER BY `+"`%s`"+` DESC
LIMIT %d
`, strings.Join(selectParts, ", "), sortColumn, limit)
}

func (c *Collector) scanProxySQLDynamicRows(rows *sql.Rows, cols []proxysqlColumn) ([][]any, error) {
	data := make([][]any, 0, 500)

	valuePtrs := make([]any, len(cols))
	values := make([]any, len(cols))

	for rows.Next() {
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
						s = strmutil.TruncateText(s, proxysqlMaxQueryTextLength)
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

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	availableCols, err := c.detectProxySQLDigestColumns(ctx)
	if err != nil {
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("failed to detect available columns: %v", err)}
	}

	cols := c.buildAvailableProxySQLColumns(availableCols)
	if len(cols) == 0 {
		return &funcapi.FunctionResponse{Status: 500, Message: "no columns available in stats_mysql_query_digest"}
	}

	cs := proxysqlColumnSet(cols)
	sortColumn = c.mapAndValidateProxySQLSortColumn(sortColumn, cs)

	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	query := c.buildProxySQLDynamicSQL(cols, sortColumn, limit)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &funcapi.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	data, err := c.scanProxySQLDynamicRows(rows, cols)
	if err != nil {
		return &funcapi.FunctionResponse{Status: 500, Message: err.Error()}
	}

	defaultSort := "totalTime"
	if !cs.ContainsColumn(defaultSort) {
		defaultSort = "calls"
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from ProxySQL stats_mysql_query_digest",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{buildProxySQLSortParam(cols)},
		ChartingConfig:    cs.BuildCharting(),
	}
}
