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

const topQueriesMaxTextLength = 4096

type topQueriesColumn struct {
	funcapi.ColumnMeta
	IsSortOption  bool   // whether this column appears as a sort option
	SortLabel     string // label for sort option dropdown
	IsDefaultSort bool   // default sort column
}

var topQueriesColumns = []topQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "taskId", Tooltip: "Task ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount}, IsSortOption: true, SortLabel: "Top queries by Task ID"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "node", Tooltip: "Node ID", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "nodeName", Tooltip: "Node Name", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "action", Tooltip: "Action", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "type", Tooltip: "Type", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "description", Tooltip: "Description", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "startTime", Tooltip: "Start Time", Type: funcapi.FieldTypeTimestamp, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, IsSortOption: true, SortLabel: "Top queries by Start Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "runningTime", Tooltip: "Running Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "RunningTime", Title: "Running Time", IsDefault: true}}, IsSortOption: true, IsDefaultSort: true, SortLabel: "Top queries by Running Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cancellable", Tooltip: "Cancellable", Type: funcapi.FieldTypeBoolean, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNone}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cancelled", Tooltip: "Cancelled", Type: funcapi.FieldTypeBoolean, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNone}},
}

type topQueriesResponse struct {
	Nodes map[string]struct {
		Name  string                    `json:"name"`
		Tasks map[string]topQueriesTask `json:"tasks"`
	} `json:"nodes"`
}

type topQueriesTask struct {
	ID                 int64  `json:"id"`
	Action             string `json:"action"`
	Type               string `json:"type"`
	Description        string `json:"description"`
	StartTimeInMillis  int64  `json:"start_time_in_millis"`
	RunningTimeInNanos int64  `json:"running_time_in_nanos"`
	Cancellable        bool   `json:"cancellable"`
	Cancelled          bool   `json:"cancelled"`
}

type topQueriesRow struct {
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

// funcTopQueries implements funcapi.MethodHandler for Elasticsearch top-queries.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return []funcapi.ParamConfig{buildTopQueriesSortOptions(topQueriesColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.httpClient == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	switch method {
	case "top-queries":
		return f.collectData(ctx, params.Column("__sort"))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func (f *funcTopQueries) collectData(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := f.router.collector.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	req, err := web.NewHTTPRequestWithPath(f.router.collector.RequestConfig, "/_tasks")
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}
	req = req.WithContext(ctx)
	q := url.Values{}
	q.Set("actions", "*search")
	q.Set("detailed", "true")
	req.URL.RawQuery = q.Encode()

	var resp topQueriesResponse
	if err := web.DoHTTP(f.router.collector.httpClient).RequestJSON(req, &resp); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("tasks query failed: %v", err)}
	}

	rows := make([]topQueriesRow, 0, 100)
	for nodeID, node := range resp.Nodes {
		for taskID, task := range node.Tasks {
			rows = append(rows, topQueriesRow{
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

	cs := f.columnSet(topQueriesColumns)
	sortParam := buildTopQueriesSortOptions(topQueriesColumns)

	if len(rows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running search tasks found.",
			Help:              "Running queries from Elasticsearch Tasks API",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: "runningTime",
			RequiredParams:    []funcapi.ParamConfig{sortParam},
			Charts:            cs.BuildCharts(),
			DefaultCharts:     cs.BuildDefaultCharts(),
			GroupBy:           cs.BuildGroupBy(),
		}
	}

	sortColumn = f.mapSortColumn(sortColumn)
	f.sortRows(rows, sortColumn)

	if len(rows) > limit {
		rows = rows[:limit]
	}

	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		out := make([]any, len(topQueriesColumns))
		for i, col := range topQueriesColumns {
			switch col.Name {
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
				out[i] = strmutil.TruncateText(row.Description, topQueriesMaxTextLength)
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
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "runningTime",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) mapSortColumn(col string) string {
	switch col {
	case "runningTime", "startTime", "taskId":
		return col
	default:
		return "runningTime"
	}
}

func (f *funcTopQueries) sortRows(rows []topQueriesRow, sortColumn string) {
	switch sortColumn {
	case "startTime":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].StartTime.After(rows[j].StartTime)
		})
	case "taskId":
		sort.Slice(rows, func(i, j int) bool {
			left, lok := f.parseTaskID(rows[i].TaskID)
			right, rok := f.parseTaskID(rows[j].TaskID)
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

func (f *funcTopQueries) parseTaskID(taskID string) (int64, bool) {
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

func buildTopQueriesSortOptions(cols []topQueriesColumn) funcapi.ParamConfig {
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
