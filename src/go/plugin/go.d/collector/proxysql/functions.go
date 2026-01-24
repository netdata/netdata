// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const proxysqlMaxQueryTextLength = 4096

const (
	paramSort = "__sort"

	ftString   = funcapi.FieldTypeString
	ftInteger  = funcapi.FieldTypeInteger
	ftDuration = funcapi.FieldTypeDuration

	trNone     = funcapi.FieldTransformNone
	trNumber   = funcapi.FieldTransformNumber
	trDuration = funcapi.FieldTransformDuration

	sortAsc  = funcapi.FieldSortAscending
	sortDesc = funcapi.FieldSortDescending

	summaryCount = funcapi.FieldSummaryCount
	summarySum   = funcapi.FieldSummarySum
	summaryMin   = funcapi.FieldSummaryMin
	summaryMax   = funcapi.FieldSummaryMax
	summaryMean  = funcapi.FieldSummaryMean

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

type proxysqlColumnMeta struct {
	dbColumn       string
	uiKey          string
	displayName    string
	dataType       funcapi.FieldType
	units          string
	visible        bool
	transform      funcapi.FieldTransform
	decimalPoints  int
	sortDir        funcapi.FieldSort
	summary        funcapi.FieldSummary
	filter         funcapi.FieldFilter
	isMicroseconds bool
	isSortOption   bool
	sortLabel      string
	isDefaultSort  bool
	isUniqueKey    bool
	isSticky       bool
	fullWidth      bool
	isLabel        bool
	isPrimary      bool
	isMetric       bool
	chartGroup     string
	chartTitle     string
	isDefaultChart bool
}

var proxysqlAllColumns = []proxysqlColumnMeta{
	{dbColumn: "digest", uiKey: "digest", displayName: "Digest", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isUniqueKey: true},
	{dbColumn: "digest_text", uiKey: "query", displayName: "Query", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isSticky: true, fullWidth: true},
	{dbColumn: "schemaname", uiKey: "schema", displayName: "Schema", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isLabel: true, isPrimary: true},
	{dbColumn: "username", uiKey: "user", displayName: "User", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isLabel: true},
	{dbColumn: "hostgroup", uiKey: "hostgroup", displayName: "Hostgroup", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortAsc, summary: summaryCount, filter: filterRange, isLabel: true},

	{dbColumn: "count_star", uiKey: "calls", displayName: "Calls", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Number of Calls", isMetric: true, chartGroup: "Calls", chartTitle: "Number of Calls", isDefaultChart: true},

	{dbColumn: "sum_time", uiKey: "totalTime", displayName: "Total Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isMicroseconds: true, isSortOption: true, sortLabel: "Total Execution Time", isDefaultSort: true, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time", isDefaultChart: true},
	{dbColumn: "avg_time", uiKey: "avgTime", displayName: "Avg Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, filter: filterRange, isMicroseconds: true, isSortOption: true, sortLabel: "Average Execution Time", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{dbColumn: "min_time", uiKey: "minTime", displayName: "Min Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange, isMicroseconds: true, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{dbColumn: "max_time", uiKey: "maxTime", displayName: "Max Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},

	{dbColumn: "sum_rows_affected", uiKey: "rowsAffected", displayName: "Rows Affected", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Rows Affected", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{dbColumn: "sum_rows_sent", uiKey: "rowsSent", displayName: "Rows Sent", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Rows Sent", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{dbColumn: "sum_errors", uiKey: "errors", displayName: "Errors", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Errors", isMetric: true, chartGroup: "Errors", chartTitle: "Errors & Warnings"},
	{dbColumn: "sum_warnings", uiKey: "warnings", displayName: "Warnings", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Warnings", isMetric: true, chartGroup: "Errors", chartTitle: "Errors & Warnings"},

	{dbColumn: "first_seen", uiKey: "firstSeen", displayName: "First Seen", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},
	{dbColumn: "last_seen", uiKey: "lastSeen", displayName: "Last Seen", dataType: ftString, visible: false, transform: trNone, sortDir: sortDesc, summary: summaryCount, filter: filterMulti},
}

func proxysqlMethods() []module.MethodConfig {
	sortOptions := buildProxySQLSortOptions(proxysqlAllColumns)
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from ProxySQL query digest stats",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{{
				ID:         paramSort,
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    sortOptions,
				UniqueView: true,
			}},
		},
	}
}

func proxysqlMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	if collector.db == nil {
		if err := collector.openConnection(); err != nil {
			return nil, err
		}
	}
	switch method {
	case "top-queries":
		return collector.topQueriesParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func proxysqlHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if collector.db == nil {
		if err := collector.openConnection(); err != nil {
			return &module.FunctionResponse{Status: 503, Message: fmt.Sprintf("failed to open connection: %v", err)}
		}
	}

	switch method {
	case "top-queries":
		return collector.collectTopQueries(ctx, params.Column(paramSort))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

func buildProxySQLSortOptions(cols []proxysqlColumnMeta) []funcapi.ParamOption {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if col.isSortOption {
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.uiKey,
				Column:  col.uiKey,
				Name:    "Top queries by " + col.sortLabel,
				Default: col.isDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return sortOptions
}

func buildProxySQLColumns(cols []proxysqlColumnMeta) map[string]any {
	columns := make(map[string]any, len(cols))
	for i, col := range cols {
		visual := funcapi.FieldVisualValue
		if col.dataType == ftDuration {
			visual = funcapi.FieldVisualBar
		}
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.displayName,
			Type:                  col.dataType,
			Units:                 col.units,
			Visualization:         visual,
			Sort:                  col.sortDir,
			Sortable:              true,
			Sticky:                col.isSticky,
			Summary:               col.summary,
			Filter:                col.filter,
			FullWidth:             col.fullWidth,
			Wrap:                  false,
			DefaultExpandedFilter: false,
			UniqueKey:             col.isUniqueKey,
			Visible:               col.visible,
			ValueOptions: funcapi.ValueOptions{
				Transform:     col.transform,
				DecimalPoints: col.decimalPoints,
				DefaultValue:  nil,
			},
		}
		columns[col.uiKey] = colDef.BuildColumn()
	}
	return columns
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

func (c *Collector) buildAvailableProxySQLColumns(available map[string]bool) []proxysqlColumnMeta {
	var cols []proxysqlColumnMeta
	for _, col := range proxysqlAllColumns {
		if col.dbColumn == "" || available[strings.ToLower(col.dbColumn)] {
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
	sortParam := funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildProxySQLSortOptions(cols),
		UniqueView: true,
	}
	return []funcapi.ParamConfig{sortParam}, nil
}

func (c *Collector) mapAndValidateProxySQLSortColumn(input string, available []proxysqlColumnMeta) string {
	availableKeys := make(map[string]bool, len(available))
	for _, col := range available {
		availableKeys[col.uiKey] = true
	}
	if availableKeys[input] {
		return input
	}
	if availableKeys["totalTime"] {
		return "totalTime"
	}
	if availableKeys["calls"] {
		return "calls"
	}
	return available[0].uiKey
}

func (c *Collector) buildProxySQLDynamicSQL(cols []proxysqlColumnMeta, sortColumn string, limit int) string {
	selectParts := make([]string, 0, len(cols))
	for _, col := range cols {
		expr := col.dbColumn
		if col.isMicroseconds {
			expr = fmt.Sprintf("%s/1000", col.dbColumn)
		}
		selectParts = append(selectParts, fmt.Sprintf("%s AS `%s`", expr, col.uiKey))
	}

	return fmt.Sprintf(`
SELECT %s
FROM stats_mysql_query_digest
ORDER BY `+"`%s`"+` DESC
LIMIT %d
`, strings.Join(selectParts, ", "), sortColumn, limit)
}

func (c *Collector) scanProxySQLDynamicRows(rows *sql.Rows, cols []proxysqlColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	valuePtrs := make([]any, len(cols))
	values := make([]any, len(cols))

	for rows.Next() {
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
					if col.uiKey == "query" {
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

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	availableCols, err := c.detectProxySQLDigestColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("failed to detect available columns: %v", err)}
	}

	cols := c.buildAvailableProxySQLColumns(availableCols)
	if len(cols) == 0 {
		return &module.FunctionResponse{Status: 500, Message: "no columns available in stats_mysql_query_digest"}
	}

	sortColumn = c.mapAndValidateProxySQLSortColumn(sortColumn, cols)

	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	query := c.buildProxySQLDynamicSQL(cols, sortColumn, limit)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	data, err := c.scanProxySQLDynamicRows(rows, cols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	sortParam := funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildProxySQLSortOptions(cols),
		UniqueView: true,
	}

	defaultSort := "totalTime"
	if !containsProxySQLColumn(cols, defaultSort) {
		defaultSort = "calls"
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from ProxySQL stats_mysql_query_digest",
		Columns:           buildProxySQLColumns(cols),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            proxysqlTopQueriesCharts(cols),
		DefaultCharts:     proxysqlTopQueriesDefaultCharts(cols),
		GroupBy:           proxysqlTopQueriesGroupBy(cols),
	}
}

func containsProxySQLColumn(cols []proxysqlColumnMeta, key string) bool {
	for _, col := range cols {
		if col.uiKey == key {
			return true
		}
	}
	return false
}

func proxysqlTopQueriesCharts(cols []proxysqlColumnMeta) map[string]module.ChartConfig {
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
			cfg = module.ChartConfig{
				Name: title,
				Type: "stacked-bar",
			}
		}
		cfg.Columns = append(cfg.Columns, col.uiKey)
		charts[col.chartGroup] = cfg
	}
	return charts
}

func proxysqlTopQueriesDefaultCharts(cols []proxysqlColumnMeta) [][]string {
	label := primaryProxySQLLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultProxySQLChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func proxysqlTopQueriesGroupBy(cols []proxysqlColumnMeta) map[string]module.GroupByConfig {
	groupBy := make(map[string]module.GroupByConfig)
	for _, col := range cols {
		if !col.isLabel {
			continue
		}
		groupBy[col.uiKey] = module.GroupByConfig{
			Name:    "Group by " + col.displayName,
			Columns: []string{col.uiKey},
		}
	}
	return groupBy
}

func primaryProxySQLLabel(cols []proxysqlColumnMeta) string {
	for _, col := range cols {
		if col.isPrimary {
			return col.uiKey
		}
	}
	for _, col := range cols {
		if col.isLabel {
			return col.uiKey
		}
	}
	return ""
}

func defaultProxySQLChartGroups(cols []proxysqlColumnMeta) []string {
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
