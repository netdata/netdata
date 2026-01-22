// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const clickhouseMaxQueryTextLength = 4096

const (
	paramSort = "__sort"

	ftString   = funcapi.FieldTypeString
	ftInteger  = funcapi.FieldTypeInteger
	ftFloat    = funcapi.FieldTypeFloat
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

type clickhouseColumnMeta struct {
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
	isSortOption   bool
	sortLabel      string
	isDefaultSort  bool
	isUniqueKey    bool
	isSticky       bool
	fullWidth      bool
	selectExpr     string
	isLabel        bool
	isPrimary      bool
	isMetric       bool
	chartGroup     string
	chartTitle     string
	isDefaultChart bool
}

var clickhouseAllColumns = []clickhouseColumnMeta{
	{dbColumn: "normalized_query_hash", uiKey: "queryId", displayName: "Query ID", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isUniqueKey: true, selectExpr: "toString(normalized_query_hash)"},
	{dbColumn: "query", uiKey: "query", displayName: "Query", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isSticky: true, fullWidth: true, selectExpr: "any(query)"},
	{dbColumn: "current_database", uiKey: "database", displayName: "Database", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, selectExpr: "any(current_database)", isLabel: true, isPrimary: true},
	{dbColumn: "user", uiKey: "user", displayName: "User", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, selectExpr: "any(user)", isLabel: true},

	{dbColumn: "", uiKey: "calls", displayName: "Calls", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Number of Calls", selectExpr: "count()", isMetric: true, chartGroup: "Calls", chartTitle: "Number of Calls", isDefaultChart: true},

	{dbColumn: "query_duration_ms", uiKey: "totalTime", displayName: "Total Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Total Execution Time", isDefaultSort: true, selectExpr: "sum(query_duration_ms)", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time", isDefaultChart: true},
	{dbColumn: "query_duration_ms", uiKey: "avgTime", displayName: "Avg Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, filter: filterRange, isSortOption: true, sortLabel: "Average Execution Time", selectExpr: "avg(query_duration_ms)", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{dbColumn: "query_duration_ms", uiKey: "minTime", displayName: "Min Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange, selectExpr: "min(query_duration_ms)", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{dbColumn: "query_duration_ms", uiKey: "maxTime", displayName: "Max Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, selectExpr: "max(query_duration_ms)", isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},

	{dbColumn: "read_rows", uiKey: "readRows", displayName: "Read Rows", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Rows Read", selectExpr: "sum(read_rows)", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{dbColumn: "read_bytes", uiKey: "readBytes", displayName: "Read Bytes", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, selectExpr: "sum(read_bytes)", isMetric: true, chartGroup: "Bytes", chartTitle: "Bytes"},
	{dbColumn: "written_rows", uiKey: "writtenRows", displayName: "Written Rows", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, selectExpr: "sum(written_rows)", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{dbColumn: "written_bytes", uiKey: "writtenBytes", displayName: "Written Bytes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, selectExpr: "sum(written_bytes)", isMetric: true, chartGroup: "Bytes", chartTitle: "Bytes"},
	{dbColumn: "result_rows", uiKey: "resultRows", displayName: "Result Rows", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, selectExpr: "sum(result_rows)", isMetric: true, chartGroup: "Rows", chartTitle: "Rows"},
	{dbColumn: "result_bytes", uiKey: "resultBytes", displayName: "Result Bytes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, selectExpr: "sum(result_bytes)", isMetric: true, chartGroup: "Bytes", chartTitle: "Bytes"},
	{dbColumn: "memory_usage", uiKey: "memoryUsage", displayName: "Max Memory", dataType: ftFloat, visible: false, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMax, filter: filterRange, selectExpr: "max(memory_usage)", isMetric: true, chartGroup: "Memory", chartTitle: "Memory"},
}

type clickhouseJSONResponse struct {
	Data []map[string]any `json:"data"`
}

func clickhouseMethods() []module.MethodConfig {
	sortOptions := buildClickHouseSortOptions(clickhouseAllColumns)
	return []module.MethodConfig{{
		ID:   "top-queries",
		Name: "Top Queries",
		Help: "Top SQL queries from ClickHouse system.query_log",
		RequiredParams: []funcapi.ParamConfig{{
			ID:         paramSort,
			Name:       "Filter By",
			Help:       "Select the primary sort column",
			Selection:  funcapi.ParamSelect,
			Options:    sortOptions,
			UniqueView: true,
		}},
	}}
}

func clickhouseMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	if collector.httpClient == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case "top-queries":
		return collector.topQueriesParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func clickhouseHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if collector.httpClient == nil {
		return &module.FunctionResponse{
			Status:  503,
			Message: "collector is still initializing, please retry in a few seconds",
		}
	}

	switch method {
	case "top-queries":
		return collector.collectTopQueries(ctx, params.Column(paramSort))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

func buildClickHouseSortOptions(cols []clickhouseColumnMeta) []funcapi.ParamOption {
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

func (c *Collector) topQueriesParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	available, err := c.detectQueryLogColumns(ctx)
	if err != nil {
		return nil, err
	}
	cols := c.buildAvailableClickHouseColumns(available)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in system.query_log")
	}
	sortParam := funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildClickHouseSortOptions(cols),
		UniqueView: true,
	}
	return []funcapi.ParamConfig{sortParam}, nil
}

func (c *Collector) detectQueryLogColumns(ctx context.Context) (map[string]bool, error) {
	query := `
SELECT name
FROM system.columns
WHERE database = 'system' AND table = 'query_log'
FORMAT JSON`

	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.URL.RawQuery = makeURLQuery(query)

	var resp clickhouseJSONResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("failed to query system.columns: %w", err)
	}

	cols := make(map[string]bool, len(resp.Data))
	for _, row := range resp.Data {
		if name, ok := row["name"].(string); ok {
			cols[name] = true
		}
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("system.query_log not available")
	}
	return cols, nil
}

func (c *Collector) buildAvailableClickHouseColumns(available map[string]bool) []clickhouseColumnMeta {
	var cols []clickhouseColumnMeta
	for _, col := range clickhouseAllColumns {
		if col.dbColumn == "" || available[col.dbColumn] {
			cols = append(cols, col)
		}
	}
	return cols
}

func (c *Collector) mapAndValidateClickHouseSortColumn(input string, available []clickhouseColumnMeta) string {
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

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	availableCols, err := c.detectQueryLogColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{Status: 503, Message: fmt.Sprintf("system.query_log not available: %v", err)}
	}

	cols := c.buildAvailableClickHouseColumns(availableCols)
	if len(cols) == 0 {
		return &module.FunctionResponse{Status: 500, Message: "no columns available in system.query_log"}
	}

	sortColumn = c.mapAndValidateClickHouseSortColumn(sortColumn, cols)

	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	groupKey := "normalized_query_hash"
	if !availableCols[groupKey] {
		groupKey = "query"
	}

	selectParts := make([]string, 0, len(cols))
	for _, col := range cols {
		selectParts = append(selectParts, fmt.Sprintf("%s AS `%s`", col.selectExpr, col.uiKey))
	}

	query := fmt.Sprintf(`
SELECT %s
FROM system.query_log
WHERE type = 'QueryFinish'
GROUP BY %s
ORDER BY `+"`%s`"+` DESC
LIMIT %d
FORMAT JSON
`, strings.Join(selectParts, ", "), groupKey, sortColumn, limit)

	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}
	req = req.WithContext(ctx)
	req.URL.RawQuery = makeURLQuery(query)

	var resp clickhouseJSONResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}

	data := make([][]any, 0, len(resp.Data))
	for _, rowMap := range resp.Data {
		row := make([]any, len(cols))
		for i, col := range cols {
			row[i] = normalizeClickHouseValue(col, rowMap[col.uiKey])
		}
		data = append(data, row)
	}

	sortParam := funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildClickHouseSortOptions(cols),
		UniqueView: true,
	}

	defaultSort := "totalTime"
	if !containsClickHouseColumn(cols, defaultSort) {
		defaultSort = "calls"
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from ClickHouse system.query_log",
		Columns:           buildClickHouseColumns(cols),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            clickhouseTopQueriesCharts(cols),
		DefaultCharts:     clickhouseTopQueriesDefaultCharts(cols),
		GroupBy:           clickhouseTopQueriesGroupBy(cols),
	}
}

func normalizeClickHouseValue(col clickhouseColumnMeta, v any) any {
	switch col.dataType {
	case ftInteger:
		switch val := v.(type) {
		case float64:
			return int64(val)
		case json.Number:
			if i, err := val.Int64(); err == nil {
				return i
			}
		case string:
			if i, err := strconv.ParseInt(val, 10, 64); err == nil {
				return i
			}
		}
		return int64(0)
	case ftFloat, ftDuration:
		switch val := v.(type) {
		case float64:
			return val
		case json.Number:
			if f, err := val.Float64(); err == nil {
				return f
			}
		case string:
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return f
			}
		}
		return float64(0)
	default:
		if s, ok := v.(string); ok {
			if col.uiKey == "query" {
				return strmutil.TruncateText(s, clickhouseMaxQueryTextLength)
			}
			return s
		}
		if v == nil {
			return ""
		}
		if col.uiKey == "query" {
			return strmutil.TruncateText(fmt.Sprint(v), clickhouseMaxQueryTextLength)
		}
		return fmt.Sprint(v)
	}
}

func buildClickHouseColumns(cols []clickhouseColumnMeta) map[string]any {
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

func containsClickHouseColumn(cols []clickhouseColumnMeta, key string) bool {
	for _, col := range cols {
		if col.uiKey == key {
			return true
		}
	}
	return false
}

func clickhouseTopQueriesCharts(cols []clickhouseColumnMeta) map[string]module.ChartConfig {
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

func clickhouseTopQueriesDefaultCharts(cols []clickhouseColumnMeta) [][]string {
	label := primaryClickhouseLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultClickhouseChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func clickhouseTopQueriesGroupBy(cols []clickhouseColumnMeta) map[string]module.GroupByConfig {
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

func primaryClickhouseLabel(cols []clickhouseColumnMeta) string {
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

func defaultClickhouseChartGroups(cols []clickhouseColumnMeta) []string {
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
