// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const elasticMaxQueryTextLength = 4096

const (
	paramSort = "__sort"

	ftString    = funcapi.FieldTypeString
	ftDuration  = funcapi.FieldTypeDuration
	ftTimestamp = funcapi.FieldTypeTimestamp
	ftBoolean   = funcapi.FieldTypeBoolean

	trNone     = funcapi.FieldTransformNone
	trDuration = funcapi.FieldTransformDuration
	trDatetime = funcapi.FieldTransformDatetime
	trText     = funcapi.FieldTransformText

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

type esColumnMeta struct {
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

var esAllColumns = []esColumnMeta{
	{id: "taskId", name: "Task ID", colType: ftString, visible: false, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, uniqueKey: true, sortDir: sortDesc, summary: summaryCount},
	{id: "node", name: "Node ID", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "nodeName", name: "Node Name", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true, isPrimary: true},
	{id: "action", name: "Action", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "type", name: "Type", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "description", name: "Description", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "startTime", name: "Start Time", colType: ftTimestamp, visible: true, sortable: true, filter: filterRange, visualization: visValue, transform: trDatetime, sortDir: sortDesc, summary: summaryMax},
	{id: "runningTime", name: "Running Time", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "RunningTime", chartTitle: "Running Time", isDefaultChart: true},
	{id: "cancellable", name: "Cancellable", colType: ftBoolean, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trNone},
	{id: "cancelled", name: "Cancelled", colType: ftBoolean, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trNone},
}

type esTasksResponse struct {
	Nodes map[string]struct {
		Name  string            `json:"name"`
		Tasks map[string]esTask `json:"tasks"`
	} `json:"nodes"`
}

type esTask struct {
	ID                 int64  `json:"id"`
	Action             string `json:"action"`
	Type               string `json:"type"`
	Description        string `json:"description"`
	StartTimeInMillis  int64  `json:"start_time_in_millis"`
	RunningTimeInNanos int64  `json:"running_time_in_nanos"`
	Cancellable        bool   `json:"cancellable"`
	Cancelled          bool   `json:"cancelled"`
}

type esTaskRow struct {
	TaskID      string
	NodeID      string
	NodeName    string
	Action      string
	Type        string
	Description string
	StartTime   time.Time
	RunningTime time.Duration
	Cancellable bool
	Cancelled   bool
}

func elasticsearchMethods() []module.MethodConfig {
	sortOptions := buildElasticsearchSortOptions(esAllColumns)
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Running queries from Elasticsearch Tasks API",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				{
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

func elasticsearchMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return []funcapi.ParamConfig{buildElasticsearchSortParam(esAllColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func elasticsearchHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
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

func buildElasticsearchSortOptions(cols []esColumnMeta) []funcapi.ParamOption {
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
		if col.id == "runningTime" {
			opt.Default = true
		}
		sortOptions = append(sortOptions, opt)
	}
	return sortOptions
}

func buildElasticsearchSortParam(cols []esColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildElasticsearchSortOptions(cols),
		UniqueView: true,
	}
}

func buildElasticsearchColumns(cols []esColumnMeta) map[string]any {
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

	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, "/_tasks")
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}
	req = req.WithContext(ctx)
	q := url.Values{}
	q.Set("actions", "*search")
	q.Set("detailed", "true")
	req.URL.RawQuery = q.Encode()

	var resp esTasksResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("tasks query failed: %v", err)}
	}

	rows := make([]esTaskRow, 0, 100)
	for nodeID, node := range resp.Nodes {
		for taskID, task := range node.Tasks {
			rows = append(rows, esTaskRow{
				TaskID:      taskID,
				NodeID:      nodeID,
				NodeName:    node.Name,
				Action:      task.Action,
				Type:        task.Type,
				Description: task.Description,
				StartTime:   time.UnixMilli(task.StartTimeInMillis),
				RunningTime: time.Duration(task.RunningTimeInNanos),
				Cancellable: task.Cancellable,
				Cancelled:   task.Cancelled,
			})
		}
	}

	if len(rows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running search tasks found.",
			Help:              "Running queries from Elasticsearch Tasks API",
			Columns:           buildElasticsearchColumns(esAllColumns),
			Data:              [][]any{},
			DefaultSortColumn: "runningTime",
			RequiredParams:    []funcapi.ParamConfig{buildElasticsearchSortParam(esAllColumns)},
			Charts:            elasticsearchTopQueriesCharts(esAllColumns),
			DefaultCharts:     elasticsearchTopQueriesDefaultCharts(esAllColumns),
			GroupBy:           elasticsearchTopQueriesGroupBy(esAllColumns),
		}
	}

	sortColumn = mapElasticsearchSortColumn(sortColumn)
	sortElasticsearchRows(rows, sortColumn)

	if len(rows) > limit {
		rows = rows[:limit]
	}

	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		out := make([]any, len(esAllColumns))
		for i, col := range esAllColumns {
			switch col.id {
			case "taskId":
				out[i] = row.TaskID
			case "node":
				out[i] = row.NodeID
			case "nodeName":
				out[i] = row.NodeName
			case "action":
				out[i] = row.Action
			case "type":
				out[i] = row.Type
			case "description":
				out[i] = strmutil.TruncateText(row.Description, elasticMaxQueryTextLength)
			case "startTime":
				out[i] = row.StartTime.Format(time.RFC3339Nano)
			case "runningTime":
				out[i] = float64(row.RunningTime) / float64(time.Millisecond)
			case "cancellable":
				out[i] = row.Cancellable
			case "cancelled":
				out[i] = row.Cancelled
			default:
				out[i] = nil
			}
		}
		data = append(data, out)
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Running queries from Elasticsearch Tasks API",
		Columns:           buildElasticsearchColumns(esAllColumns),
		Data:              data,
		DefaultSortColumn: "runningTime",
		RequiredParams:    []funcapi.ParamConfig{buildElasticsearchSortParam(esAllColumns)},
		Charts:            elasticsearchTopQueriesCharts(esAllColumns),
		DefaultCharts:     elasticsearchTopQueriesDefaultCharts(esAllColumns),
		GroupBy:           elasticsearchTopQueriesGroupBy(esAllColumns),
	}
}

func mapElasticsearchSortColumn(col string) string {
	switch col {
	case "runningTime", "startTime", "taskId":
		return col
	default:
		return "runningTime"
	}
}

func sortElasticsearchRows(rows []esTaskRow, sortColumn string) {
	switch sortColumn {
	case "startTime":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].StartTime.After(rows[j].StartTime)
		})
	case "taskId":
		sort.Slice(rows, func(i, j int) bool {
			left, lok := parseElasticsearchTaskID(rows[i].TaskID)
			right, rok := parseElasticsearchTaskID(rows[j].TaskID)
			if lok && rok {
				return left > right
			}
			return rows[i].TaskID > rows[j].TaskID
		})
	default:
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].RunningTime > rows[j].RunningTime
		})
	}
}

func parseElasticsearchTaskID(taskID string) (int64, bool) {
	id := taskID
	if idx := strings.LastIndex(id, ":"); idx != -1 {
		id = id[idx+1:]
	}
	val, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

func elasticsearchTopQueriesCharts(cols []esColumnMeta) map[string]module.ChartConfig {
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

func elasticsearchTopQueriesDefaultCharts(cols []esColumnMeta) [][]string {
	label := primaryElasticsearchLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultElasticsearchChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func elasticsearchTopQueriesGroupBy(cols []esColumnMeta) map[string]module.GroupByConfig {
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

func primaryElasticsearchLabel(cols []esColumnMeta) string {
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

func defaultElasticsearchChartGroups(cols []esColumnMeta) []string {
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
