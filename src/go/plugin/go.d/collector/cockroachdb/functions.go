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

var errSQLDSNNotSet = errors.New("SQL DSN is not set")

type crdbColumnMeta struct {
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
	isLabel        bool
	isPrimary      bool
	isMetric       bool
	chartGroup     string
	chartTitle     string
	isDefaultChart bool
}

var crdbTopColumns = []crdbColumnMeta{
	{id: "fingerprintId", name: "Fingerprint ID", selectExpr: "s.fingerprint_id::STRING", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", selectExpr: "s.metadata->>'query'", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "database", name: "Database", selectExpr: "s.metadata->>'db'", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, isLabel: true, isPrimary: true},
	{id: "application", name: "Application", selectExpr: "s.app_name", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, isLabel: true},
	{id: "statementType", name: "Statement Type", selectExpr: "s.metadata->>'stmtTyp'", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount, isLabel: true},
	{id: "distributed", name: "Distributed", selectExpr: "s.metadata->>'distsql'", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "fullScan", name: "Full Scan", selectExpr: "s.metadata->>'fullScan'", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "implicitTxn", name: "Implicit Txn", selectExpr: "s.metadata->>'implicitTxn'", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "vectorized", name: "Vectorized", selectExpr: "s.metadata->>'vec'", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},

	{id: "executions", name: "Executions", selectExpr: "COALESCE((s.statistics->'statistics'->>'cnt')::INT8, 0)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Executions", isMetric: true, chartGroup: "Calls", chartTitle: "Executions", isDefaultChart: true},
	{id: "totalTime", name: "Total Time", selectExpr: "COALESCE((s.statistics->'statistics'->'svcLat'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0) * 1000", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isSortOption: true, isDefaultSort: true, sortLabel: "Top queries by Total Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time", isDefaultChart: true},
	{id: "meanTime", name: "Mean Time", selectExpr: "COALESCE((s.statistics->'statistics'->'svcLat'->>'mean')::FLOAT8, 0) * 1000", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, isSortOption: true, sortLabel: "Top queries by Mean Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "runTime", name: "Run Time", selectExpr: "COALESCE((s.statistics->'statistics'->'runLat'->>'mean')::FLOAT8, 0) * 1000", dataType: ftDuration, visible: false, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "planTime", name: "Plan Time", selectExpr: "COALESCE((s.statistics->'statistics'->'planLat'->>'mean')::FLOAT8, 0) * 1000", dataType: ftDuration, visible: false, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "parseTime", name: "Parse Time", selectExpr: "COALESCE((s.statistics->'statistics'->'parseLat'->>'mean')::FLOAT8, 0) * 1000", dataType: ftDuration, visible: false, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},

	{id: "rowsRead", name: "Rows Read", selectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'rowsRead'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Rows Read", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{id: "rowsWritten", name: "Rows Written", selectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'rowsWritten'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Rows Written", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{id: "rowsReturned", name: "Rows Returned", selectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'numRows'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", dataType: ftInteger, visible: true, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Rows Returned", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{id: "bytesRead", name: "Bytes Read", selectExpr: "CAST(ROUND(COALESCE((s.statistics->'statistics'->'bytesRead'->>'mean')::FLOAT8, 0) * COALESCE((s.statistics->'statistics'->>'cnt')::FLOAT8, 0)) AS INT8)", dataType: ftInteger, visible: false, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summarySum, isSortOption: true, sortLabel: "Top queries by Bytes Read", isMetric: true, chartGroup: "Bytes", chartTitle: "Bytes"},
	{id: "maxRetries", name: "Max Retries", selectExpr: "COALESCE((s.statistics->'statistics'->>'maxRetries')::INT8, 0)", dataType: ftInteger, visible: false, sortable: true, filter: filterRange, transform: trNumber, sortDir: sortDesc, summary: summaryMax, isMetric: true, chartGroup: "Retries", chartTitle: "Retries"},
}

var crdbRunningColumns = []crdbColumnMeta{
	{id: "queryId", name: "Query ID", selectExpr: "s.query_id::STRING", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", selectExpr: "s.query", dataType: ftString, visible: true, sortable: false, filter: filterMulti, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "user", name: "User", selectExpr: "s.user_name", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "application", name: "Application", selectExpr: "s.application_name", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "clientAddress", name: "Client Address", selectExpr: "s.client_address", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "nodeId", name: "Node ID", selectExpr: "s.node_id::STRING", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "sessionId", name: "Session ID", selectExpr: "s.session_id::STRING", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "phase", name: "Phase", selectExpr: "s.phase", dataType: ftString, visible: true, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "distributed", name: "Distributed", selectExpr: "s.distributed::STRING", dataType: ftString, visible: false, sortable: true, filter: filterMulti, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "startTime", name: "Start Time", selectExpr: "TO_CHAR(s.start, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF3')", dataType: ftString, visible: false, sortable: true, filter: filterRange, transform: trText, sortDir: sortDesc, summary: summaryMax},
	{id: "elapsedMs", name: "Elapsed", selectExpr: "EXTRACT(EPOCH FROM (clock_timestamp() - s.start)) * 1000", dataType: ftDuration, visible: true, sortable: true, filter: filterRange, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isSortOption: true, isDefaultSort: true, sortLabel: "Running queries by Elapsed Time"},
}

func cockroachMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery: 10,
			ID:          "top-queries",
			Name:        "Top Queries",
			Help:        "Top SQL statements from crdb_internal.cluster_statement_statistics. WARNING: Query text may contain unmasked literals (potential PII).",
			RequiredParams: []funcapi.ParamConfig{
				buildCrdbSortParam(crdbTopColumns),
			},
		},
		{
			UpdateEvery: 10,
			ID:          "running-queries",
			Name:        "Running Queries",
			Help:        "Currently running SQL statements from SHOW CLUSTER STATEMENTS. WARNING: Query text may contain unmasked literals (potential PII).",
			RequiredParams: []funcapi.ParamConfig{
				buildCrdbSortParam(crdbRunningColumns),
			},
		},
	}
}

func cockroachMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return []funcapi.ParamConfig{buildCrdbSortParam(crdbTopColumns)}, nil
	case "running-queries":
		return []funcapi.ParamConfig{buildCrdbSortParam(crdbRunningColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func cockroachHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if err := collector.ensureSQL(ctx); err != nil {
		status := 503
		if errors.Is(err, errSQLDSNNotSet) {
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

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL statements from crdb_internal.cluster_statement_statistics. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildCrdbColumns(crdbTopColumns),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildCrdbSortParam(crdbTopColumns)},
		Charts:            crdbTopQueriesCharts(crdbTopColumns),
		DefaultCharts:     crdbTopQueriesDefaultCharts(crdbTopColumns),
		GroupBy:           crdbTopQueriesGroupBy(crdbTopColumns),
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

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running SQL statements from SHOW CLUSTER STATEMENTS. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           buildCrdbColumns(crdbRunningColumns),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildCrdbSortParam(crdbRunningColumns)},
	}
}

func buildCrdbSortParam(cols []crdbColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildCrdbSortOptions(cols),
		UniqueView: true,
	}
}

func buildCrdbSortOptions(cols []crdbColumnMeta) []funcapi.ParamOption {
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

func buildCrdbColumns(cols []crdbColumnMeta) map[string]any {
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

func resolveCrdbSortColumn(cols []crdbColumnMeta, requested string) string {
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
	return ""
}

func buildCrdbTopQueriesSQL(sortColumn string, limit int) string {
	selectCols := make([]string, 0, len(crdbTopColumns))
	for _, col := range crdbTopColumns {
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.selectExpr, col.id))
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
		selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col.selectExpr, col.id))
	}
	return fmt.Sprintf(`
SELECT %s
FROM [SHOW CLUSTER STATEMENTS] AS s
ORDER BY %s DESC NULLS LAST
LIMIT $1`, strings.Join(selectCols, ", "), sortColumn)
}

func scanCrdbRows(rows *sql.Rows, cols []crdbColumnMeta) ([][]any, error) {
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

func crdbTopQueriesCharts(cols []crdbColumnMeta) map[string]module.ChartConfig {
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

func crdbTopQueriesDefaultCharts(cols []crdbColumnMeta) [][]string {
	label := primaryCrdbLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultCrdbChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func crdbTopQueriesGroupBy(cols []crdbColumnMeta) map[string]module.GroupByConfig {
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

func primaryCrdbLabel(cols []crdbColumnMeta) string {
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

func defaultCrdbChartGroups(cols []crdbColumnMeta) []string {
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
