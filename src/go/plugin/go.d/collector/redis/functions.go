// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const redisMaxQueryTextLength = 4096

const (
	paramSort = "__sort"

	ftString    = funcapi.FieldTypeString
	ftInteger   = funcapi.FieldTypeInteger
	ftDuration  = funcapi.FieldTypeDuration
	ftTimestamp = funcapi.FieldTypeTimestamp
	trNone      = funcapi.FieldTransformNone
	trNumber    = funcapi.FieldTransformNumber
	trDuration  = funcapi.FieldTransformDuration
	trDatetime  = funcapi.FieldTransformDatetime
	trText      = funcapi.FieldTransformText

	visValue = funcapi.FieldVisualValue
	visBar   = funcapi.FieldVisualBar

	sortAsc  = funcapi.FieldSortAscending
	sortDesc = funcapi.FieldSortDescending

	summaryCount = funcapi.FieldSummaryCount
	summaryMax   = funcapi.FieldSummaryMax
	summarySum   = funcapi.FieldSummarySum

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

type redisColumnMeta struct {
	id             string
	name           string
	colType        funcapi.FieldType
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
	isLabel        bool
	isPrimary      bool
	isMetric       bool
	chartGroup     string
	chartTitle     string
	isDefaultChart bool
}

var redisAllColumns = []redisColumnMeta{
	{id: "id", name: "ID", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, transform: trNumber, uniqueKey: true, sortDir: sortDesc, summary: summaryCount},
	{id: "timestamp", name: "Timestamp", colType: ftTimestamp, visible: true, sortable: true, filter: filterRange, visualization: visValue, transform: trDatetime, sortDir: sortDesc, summary: summaryMax},
	{id: "command", name: "Command", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "command_name", name: "Command Name", colType: ftString, visible: true, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, sortDir: sortAsc, isLabel: true, isPrimary: true},
	{id: "duration", name: "Duration", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "Duration", chartTitle: "Execution Time", isDefaultChart: true},
	{id: "client_addr", name: "Client Address", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "client_name", name: "Client Name", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
}

func redisMethods() []module.MethodConfig {
	sortOptions := buildRedisSortOptions(redisAllColumns)
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII).",
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

func redisMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return []funcapi.ParamConfig{buildRedisSortParam(redisAllColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func redisHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if collector.rdb == nil {
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

func buildRedisSortOptions(cols []redisColumnMeta) []funcapi.ParamOption {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.sortable {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.id,
			Column: col.id,
			Name:   fmt.Sprintf("Top queries by %s", col.name),
			Sort:   &sortDir,
		}
		if col.id == "duration" {
			opt.Default = true
		}
		sortOptions = append(sortOptions, opt)
	}
	return sortOptions
}

func buildRedisSortParam(cols []redisColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildRedisSortOptions(cols),
		UniqueView: true,
	}
}

func buildRedisColumns(cols []redisColumnMeta) map[string]any {
	result := make(map[string]any, len(cols))
	for i, col := range cols {
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.name,
			Type:                  col.colType,
			Units:                 col.units,
			Visualization:         col.visualization,
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

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	entries, err := c.rdb.SlowLogGet(ctx, -1).Result()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("slowlog query failed: %v", err)}
	}

	if len(entries) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No slow commands found. SLOWLOG may be empty or disabled.",
			Help:              "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII).",
			Columns:           buildRedisColumns(redisAllColumns),
			Data:              [][]any{},
			DefaultSortColumn: "duration",
			RequiredParams:    []funcapi.ParamConfig{buildRedisSortParam(redisAllColumns)},
			Charts:            redisTopQueriesCharts(redisAllColumns),
			DefaultCharts:     redisTopQueriesDefaultCharts(redisAllColumns),
			GroupBy:           redisTopQueriesGroupBy(redisAllColumns),
		}
	}

	sortColumn = mapRedisSortColumn(sortColumn)
	sortRedisSlowLogs(entries, sortColumn)

	if len(entries) > limit {
		entries = entries[:limit]
	}

	data := make([][]any, 0, len(entries))
	for _, entry := range entries {
		command := strings.Join(entry.Args, " ")
		commandName := ""
		if len(entry.Args) > 0 {
			commandName = entry.Args[0]
		}

		row := make([]any, len(redisAllColumns))
		for i, col := range redisAllColumns {
			switch col.id {
			case "id":
				row[i] = entry.ID
			case "timestamp":
				row[i] = entry.Time.Format(time.RFC3339Nano)
			case "command":
				row[i] = strmutil.TruncateText(command, redisMaxQueryTextLength)
			case "command_name":
				row[i] = commandName
			case "duration":
				row[i] = float64(entry.Duration) / float64(time.Millisecond)
			case "client_addr":
				row[i] = entry.ClientAddr
			case "client_name":
				row[i] = entry.ClientName
			default:
				row[i] = nil
			}
		}
		data = append(data, row)
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII).",
		Columns:           buildRedisColumns(redisAllColumns),
		Data:              data,
		DefaultSortColumn: "duration",
		RequiredParams:    []funcapi.ParamConfig{buildRedisSortParam(redisAllColumns)},
		Charts:            redisTopQueriesCharts(redisAllColumns),
		DefaultCharts:     redisTopQueriesDefaultCharts(redisAllColumns),
		GroupBy:           redisTopQueriesGroupBy(redisAllColumns),
	}
}

func mapRedisSortColumn(col string) string {
	switch col {
	case "duration", "timestamp", "id", "command_name":
		return col
	default:
		return "duration"
	}
}

func sortRedisSlowLogs(entries []redis.SlowLog, sortColumn string) {
	switch sortColumn {
	case "timestamp":
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Time.After(entries[j].Time)
		})
	case "id":
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ID > entries[j].ID
		})
	case "command_name":
		sort.Slice(entries, func(i, j int) bool {
			var a, b string
			if len(entries[i].Args) > 0 {
				a = entries[i].Args[0]
			}
			if len(entries[j].Args) > 0 {
				b = entries[j].Args[0]
			}
			return a > b
		})
	default:
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Duration > entries[j].Duration
		})
	}
}

func redisTopQueriesCharts(cols []redisColumnMeta) map[string]module.ChartConfig {
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
		cfg.Columns = append(cfg.Columns, col.id)
		charts[col.chartGroup] = cfg
	}
	return charts
}

func redisTopQueriesDefaultCharts(cols []redisColumnMeta) [][]string {
	label := primaryRedisLabel(cols)
	if label == "" {
		return nil
	}

	chartGroups := defaultRedisChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func redisTopQueriesGroupBy(cols []redisColumnMeta) map[string]module.GroupByConfig {
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

func primaryRedisLabel(cols []redisColumnMeta) string {
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

func defaultRedisChartGroups(cols []redisColumnMeta) []string {
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
