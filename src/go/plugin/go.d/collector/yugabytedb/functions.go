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

const (
	paramSort = "__sort"

	ftString   = funcapi.FieldTypeString
	ftInteger  = funcapi.FieldTypeInteger
	ftFloat    = funcapi.FieldTypeFloat
	ftDuration = funcapi.FieldTypeDuration

	trNone     = funcapi.FieldTransformNone
	trNumber   = funcapi.FieldTransformNumber
	trDuration = funcapi.FieldTransformDuration
	trText     = funcapi.FieldTransformText

	visValue = funcapi.FieldVisualValue
	visBar   = funcapi.FieldVisualBar

	sortAsc  = funcapi.FieldSortAscending
	sortDesc = funcapi.FieldSortDescending

	summaryCount = funcapi.FieldSummaryCount
	summarySum   = funcapi.FieldSummarySum
	summaryMax   = funcapi.FieldSummaryMax
	summaryMean  = funcapi.FieldSummaryMean

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

var errYBSQLDSNNotSet = errors.New("SQL DSN is not set")

type ybColumnMeta struct {
	id             string
	name           string
	selectExpr     string
	dataType       funcapi.FieldType
	visible        bool
	sortable       bool
	fullWidth      bool
	wrap           bool
	sticky         bool
	filter         funcapi.FieldFilter
	visualization  funcapi.FieldVisual
	transform      funcapi.FieldTransform
	units          string
	decimalPoints  int
	uniqueKey      bool
	sortDir        funcapi.FieldSort
	summary        funcapi.FieldSummary
	sortLabel      string
	isSortOption   bool
	isDefaultSort  bool
	isJoinColumn   bool
	isLabel        bool
	isPrimary      bool
	isMetric       bool
	chartGroup     string
	chartTitle     string
	isDefaultChart bool
}

var ybTopColumns = []ybColumnMeta{
	{id: "queryId", name: "Query ID", selectExpr: "s.queryid::text", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", selectExpr: "s.query", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "database", name: "Database", selectExpr: "d.datname", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, isJoinColumn: true, isLabel: true, isPrimary: true},
	{id: "user", name: "User", selectExpr: "u.usename", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, isJoinColumn: true, isLabel: true},

	{id: "calls", name: "Calls", selectExpr: "s.calls", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Calls", isMetric: true, chartGroup: "Calls", chartTitle: "Number of Calls", isDefaultChart: true},
	{id: "totalTime", name: "Total Time", selectExpr: "s.total_time", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isSortOption: true, isDefaultSort: true, sortLabel: "Top queries by Total Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time", isDefaultChart: true},
	{id: "meanTime", name: "Mean Time", selectExpr: "s.mean_time", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, isSortOption: true, sortLabel: "Top queries by Mean Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "minTime", name: "Min Time", selectExpr: "s.min_time", dataType: ftDuration, visible: false, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "maxTime", name: "Max Time", selectExpr: "s.max_time", dataType: ftDuration, visible: false, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isSortOption: true, sortLabel: "Top queries by Max Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "rows", name: "Rows", selectExpr: "s.rows", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Rows Returned", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{id: "stddevTime", name: "Stddev Time", selectExpr: "s.stddev_time", dataType: ftDuration, visible: false, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
}

var ybRunningColumns = []ybColumnMeta{
	{id: "pid", name: "PID", selectExpr: "s.pid::text", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", selectExpr: "s.query", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "database", name: "Database", selectExpr: "s.datname", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "user", name: "User", selectExpr: "s.usename", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "state", name: "State", selectExpr: "s.state", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "waitEventType", name: "Wait Event Type", selectExpr: "s.wait_event_type", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "waitEvent", name: "Wait Event", selectExpr: "s.wait_event", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "application", name: "Application", selectExpr: "s.application_name", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "clientAddress", name: "Client Address", selectExpr: "s.client_addr::text", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "queryStart", name: "Query Start", selectExpr: "TO_CHAR(s.query_start, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')", dataType: ftString, visible: false, sortable: true, filter: filterRange, transform: trText, sortDir: sortDesc, summary: summaryMax},
	{id: "elapsedMs", name: "Elapsed", selectExpr: "CASE WHEN s.query_start IS NULL THEN 0 ELSE EXTRACT(EPOCH FROM (clock_timestamp() - s.query_start)) * 1000 END", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isSortOption: true, isDefaultSort: true, sortLabel: "Running queries by Elapsed Time"},
}

func yugabyteMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			ID:   "top-queries",
			Name: "Top Queries",
			Help: "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
			RequiredParams: []funcapi.ParamConfig{
				buildYBSortParam(ybTopColumns),
			},
		},
		{
			ID:   "running-queries",
			Name: "Running Queries",
			Help: "Currently running SQL statements from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
			RequiredParams: []funcapi.ParamConfig{
				buildYBSortParam(ybRunningColumns),
			},
		},
	}
}

func yugabyteMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}

	switch method {
	case "top-queries":
		cols := ybTopColumns
		if collector.db != nil {
			if available, err := collector.availableTopColumns(ctx); err == nil {
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

func yugabyteHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if err := collector.ensureSQL(ctx); err != nil {
		status := 503
		if errors.Is(err, errYBSQLDSNNotSet) {
			status = 400
		}
		return &module.FunctionResponse{Status: status, Message: err.Error()}
	}

	switch method {
	case "top-queries":
		return collector.collectTopQueries(ctx, params.Column(paramSort))
	case "running-queries":
		return collector.collectRunningQueries(ctx, params.Column(paramSort))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
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

func (c *Collector) availableTopColumns(ctx context.Context) ([]ybColumnMeta, error) {
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

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildYBColumns(cols),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildYBSortParam(cols)},
		Charts:            ybTopQueriesCharts(cols),
		DefaultCharts:     ybTopQueriesDefaultCharts(cols),
		GroupBy:           ybTopQueriesGroupBy(cols),
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

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildYBColumns(ybRunningColumns),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildYBSortParam(ybRunningColumns)},
	}
}

func buildYBSortParam(cols []ybColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildYBSortOptions(cols),
		UniqueView: true,
	}
}

func buildYBSortOptions(cols []ybColumnMeta) []funcapi.ParamOption {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.isSortOption {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.id,
			Column: col.id,
			Name:   col.sortLabel,
			Sort:   &sortDir,
		}
		if col.isDefaultSort {
			opt.Default = true
		}
		sortOptions = append(sortOptions, opt)
	}
	return sortOptions
}

func buildYBColumns(cols []ybColumnMeta) map[string]any {
	result := make(map[string]any, len(cols))
	for i, col := range cols {
		visual := visValue
		if col.dataType == ftDuration {
			visual = visBar
		}
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.name,
			Type:                  col.dataType,
			Units:                 col.units,
			Visualization:         visual,
			Sort:                  col.sortDir,
			Sortable:              col.sortable,
			Sticky:                col.sticky,
			Summary:               col.summary,
			Filter:                col.filter,
			FullWidth:             col.fullWidth,
			Wrap:                  col.wrap,
			DefaultExpandedFilter: false,
			UniqueKey:             col.uniqueKey,
			Visible:               col.visible,
			ValueOptions: funcapi.ValueOptions{
				Transform:     col.transform,
				DecimalPoints: col.decimalPoints,
				DefaultValue:  nil,
			},
		}
		result[col.id] = colDef.BuildColumn()
	}
	return result
}

func resolveYBSortColumn(cols []ybColumnMeta, requested string) string {
	if requested != "" {
		for _, col := range cols {
			if col.id == requested && col.isSortOption {
				return col.id
			}
		}
	}
	for _, col := range cols {
		if col.isDefaultSort && col.isSortOption {
			return col.id
		}
	}
	for _, col := range cols {
		if col.isSortOption {
			return col.id
		}
	}
	if len(cols) > 0 {
		return cols[0].id
	}
	return ""
}

func buildYBTopQueriesSQL(cols []ybColumnMeta, sortColumn string) string {
	selectCols := make([]string, 0, len(cols))
	for _, col := range cols {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.selectExpr, col.id))
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
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.selectExpr, col.id))
	}
	return fmt.Sprintf(`
SELECT %s
FROM pg_stat_activity s
WHERE s.state IS DISTINCT FROM 'idle'
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func scanYBRows(rows *sql.Rows, cols []ybColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))

		for i, col := range cols {
			switch col.dataType {
			case ftString:
				var v sql.NullString
				values[i] = &v
			case ftInteger:
				var v sql.NullInt64
				values[i] = &v
			case ftFloat, ftDuration:
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
					if col.id == "query" {
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

func ybTopQueriesCharts(cols []ybColumnMeta) map[string]module.ChartConfig {
	charts := make(map[string]module.ChartConfig)
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" {
			continue
		}
		cfg, ok := charts[col.chartGroup]
		if !ok {
			title := col.chartTitle
			if title == "" {
				title = col.chartGroup
			}
			cfg = module.ChartConfig{Name: title, Type: "stacked-bar"}
		}
		cfg.Columns = append(cfg.Columns, col.id)
		charts[col.chartGroup] = cfg
	}
	return charts
}

func ybTopQueriesDefaultCharts(cols []ybColumnMeta) [][]string {
	label := primaryYBLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultYBChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func ybTopQueriesGroupBy(cols []ybColumnMeta) map[string]module.GroupByConfig {
	groupBy := make(map[string]module.GroupByConfig)
	for _, col := range cols {
		if !col.isLabel {
			continue
		}
		groupBy[col.id] = module.GroupByConfig{
			Name:    "Group by " + col.name,
			Columns: []string{col.id},
		}
	}
	return groupBy
}

func primaryYBLabel(cols []ybColumnMeta) string {
	for _, col := range cols {
		if col.isPrimary {
			return col.id
		}
	}
	for _, col := range cols {
		if col.isLabel {
			return col.id
		}
	}
	return ""
}

func defaultYBChartGroups(cols []ybColumnMeta) []string {
	groups := make([]string, 0)
	seen := make(map[string]bool)
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" || !col.isDefaultChart {
			continue
		}
		if !seen[col.chartGroup] {
			seen[col.chartGroup] = true
			groups = append(groups, col.chartGroup)
		}
	}
	if len(groups) > 0 {
		return groups
	}
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" {
			continue
		}
		if !seen[col.chartGroup] {
			seen[col.chartGroup] = true
			groups = append(groups, col.chartGroup)
		}
	}
	return groups
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

func (c *Collector) buildAvailableColumns(availableCols map[string]bool) []ybColumnMeta {
	result := make([]ybColumnMeta, 0, len(ybTopColumns))
	for _, col := range ybTopColumns {
		if col.isJoinColumn {
			result = append(result, col)
			continue
		}
		actual, ok := resolveYBColumn(col.selectExpr, availableCols)
		if !ok {
			continue
		}
		colCopy := col
		colCopy.selectExpr = actual
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
