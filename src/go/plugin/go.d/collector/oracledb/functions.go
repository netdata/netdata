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

const (
	paramSort = "__sort"

	ftString   = funcapi.FieldTypeString
	ftInteger  = funcapi.FieldTypeInteger
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

type oracleColumnMeta struct {
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
	requiresColumn string
	isLabel        bool
	isPrimary      bool
	isMetric       bool
	chartGroup     string
	chartTitle     string
	isDefaultChart bool
}

type oracleTopLayout struct {
	cols []oracleColumnMeta
	join string
}

var oracleTopColumns = []oracleColumnMeta{
	{id: "sqlId", name: "SQL ID", selectExpr: "s.sql_id", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", selectExpr: "s.sql_text", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "schema", name: "Schema", selectExpr: "s.parsing_schema_name", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, isLabel: true, isPrimary: true},

	{id: "executions", name: "Executions", selectExpr: "NVL(s.executions, 0)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Executions", isMetric: true, chartGroup: "Calls", chartTitle: "Executions", isDefaultChart: true},
	{id: "totalTime", name: "Total Time", selectExpr: "NVL(s.elapsed_time, 0) / 1000", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isSortOption: true, isDefaultSort: true, sortLabel: "Top queries by Total Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time", isDefaultChart: true},
	{id: "avgTime", name: "Avg Time", selectExpr: "CASE WHEN NVL(s.executions,0) = 0 THEN 0 ELSE (s.elapsed_time / s.executions) / 1000 END", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, isSortOption: true, sortLabel: "Top queries by Avg Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "cpuTime", name: "CPU Time", selectExpr: "NVL(s.cpu_time, 0) / 1000", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by CPU Time", isMetric: true, chartGroup: "CPU", chartTitle: "CPU Time"},

	{id: "bufferGets", name: "Buffer Gets", selectExpr: "NVL(s.buffer_gets, 0)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Buffer Gets", isMetric: true, chartGroup: "IO", chartTitle: "I/O"},
	{id: "diskReads", name: "Disk Reads", selectExpr: "NVL(s.disk_reads, 0)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Disk Reads", isMetric: true, chartGroup: "IO", chartTitle: "I/O"},
	{id: "rowsProcessed", name: "Rows Processed", selectExpr: "NVL(s.rows_processed, 0)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Rows Processed", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{id: "parseCalls", name: "Parse Calls", selectExpr: "NVL(s.parse_calls, 0)", dataType: ftInteger, visible: false, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Parse Calls", isMetric: true, chartGroup: "Parse", chartTitle: "Parse Calls"},

	{id: "module", name: "Module", selectExpr: "s.module", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, requiresColumn: "MODULE", isLabel: true},
	{id: "action", name: "Action", selectExpr: "s.action", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, requiresColumn: "ACTION", isLabel: true},
	{id: "lastActiveTime", name: "Last Active", selectExpr: "TO_CHAR(CAST(s.last_active_time AS TIMESTAMP), 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')", dataType: ftString, visible: false, sortable: false, filter: filterRange, transform: trText, requiresColumn: "LAST_ACTIVE_TIME"},
}

var oracleRunningColumns = []oracleColumnMeta{
	{id: "sessionId", name: "Session", selectExpr: "s.sid || ',' || s.serial#", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "username", name: "User", selectExpr: "s.username", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "status", name: "Status", selectExpr: "s.status", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "type", name: "Type", selectExpr: "s.type", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "sqlId", name: "SQL ID", selectExpr: "s.sql_id", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", selectExpr: "q.sql_text", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "lastCallMs", name: "Elapsed", selectExpr: "NVL(s.last_call_et, 0) * 1000", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isSortOption: true, isDefaultSort: true, sortLabel: "Running queries by Elapsed Time"},
	{id: "sqlExecStart", name: "SQL Exec Start", selectExpr: "TO_CHAR(CAST(s.sql_exec_start AS TIMESTAMP), 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')", dataType: ftString, visible: false, sortable: true, filter: filterRange, transform: trText, sortDir: sortDesc, summary: summaryMax},
	{id: "module", name: "Module", selectExpr: "s.module", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "action", name: "Action", selectExpr: "s.action", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "program", name: "Program", selectExpr: "s.program", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "machine", name: "Machine", selectExpr: "s.machine", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
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
		return collector.collectTopQueries(ctx, params.Column(paramSort))
	case "running-queries":
		return collector.collectRunningQueries(ctx, params.Column(paramSort))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

func buildOracleSortParam(cols []oracleColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildOracleSortOptions(cols),
		UniqueView: true,
	}
}

func buildOracleSortOptions(cols []oracleColumnMeta) []funcapi.ParamOption {
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

func buildOracleColumns(cols []oracleColumnMeta) map[string]any {
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

func buildOracleSelect(cols []oracleColumnMeta) string {
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		expr := col.selectExpr
		if expr == "" {
			expr = col.id
		}
		parts = append(parts, fmt.Sprintf("%s AS %s", expr, col.id))
	}
	return strings.Join(parts, ", ")
}

func mapOracleSortColumn(input string, cols []oracleColumnMeta) string {
	for _, col := range cols {
		if col.isSortOption && col.id == input {
			return col.id
		}
	}
	for _, col := range cols {
		if col.isDefaultSort {
			return col.id
		}
	}
	for _, col := range cols {
		if col.isSortOption {
			return col.id
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

func filterOracleColumns(cols []oracleColumnMeta, available map[string]bool) []oracleColumnMeta {
	filtered := make([]oracleColumnMeta, 0, len(cols))
	for _, col := range cols {
		if col.requiresColumn != "" {
			if available == nil {
				continue
			}
			if !available[strings.ToUpper(col.requiresColumn)] {
				continue
			}
		}
		filtered = append(filtered, col)
	}
	return filtered
}

func buildOracleTopLayout(available map[string]bool) oracleTopLayout {
	schemaExpr, schemaJoin := resolveOracleSchemaExpr(available)
	filtered := make([]oracleColumnMeta, 0, len(oracleTopColumns))
	for _, col := range oracleTopColumns {
		if col.id == "schema" {
			if schemaExpr == "" {
				continue
			}
			col.selectExpr = schemaExpr
		}
		if col.requiresColumn != "" && !available[strings.ToUpper(col.requiresColumn)] {
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
	if len(data) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No SQL statements found.",
			Help:              "Top SQL statements from V$SQLSTATS",
			Columns:           buildOracleColumns(topCols),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(topCols)},
			Charts:            oracleTopQueriesCharts(topCols),
			DefaultCharts:     oracleTopQueriesDefaultCharts(topCols),
			GroupBy:           oracleTopQueriesGroupBy(topCols),
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildOracleColumns(topCols),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(topCols)},
		Charts:            oracleTopQueriesCharts(topCols),
		DefaultCharts:     oracleTopQueriesDefaultCharts(topCols),
		GroupBy:           oracleTopQueriesGroupBy(topCols),
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

	if len(data) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running queries found.",
			Help:              "Currently running SQL statements from V$SESSION",
			Columns:           buildOracleColumns(oracleRunningColumns),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(oracleRunningColumns)},
		}
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildOracleColumns(oracleRunningColumns),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildOracleSortParam(oracleRunningColumns)},
	}
}

func scanOracleRows(rows *sql.Rows, cols []oracleColumnMeta) ([][]any, error) {
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
			case ftDuration:
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

func oracleTopQueriesCharts(cols []oracleColumnMeta) map[string]module.ChartConfig {
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

func oracleTopQueriesDefaultCharts(cols []oracleColumnMeta) [][]string {
	label := primaryOracleLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultOracleChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func oracleTopQueriesGroupBy(cols []oracleColumnMeta) map[string]module.GroupByConfig {
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

func hasOracleColumn(cols []oracleColumnMeta, id string) bool {
	for _, col := range cols {
		if col.id == id {
			return true
		}
	}
	return false
}

func primaryOracleLabel(cols []oracleColumnMeta) string {
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

func defaultOracleChartGroups(cols []oracleColumnMeta) []string {
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
