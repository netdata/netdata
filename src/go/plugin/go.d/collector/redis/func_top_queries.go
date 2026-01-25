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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	topQueriesMethodID      = "top-queries"
	redisMaxQueryTextLength = 4096
)

func topQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(redisAllColumns)},
	}
}

type redisColumn struct {
	funcapi.ColumnMeta
	sortOpt     bool   // whether this column appears as a sort option
	sortLbl     string // label for sort option dropdown
	defaultSort bool   // default sort column
}

// funcapi.SortableColumn interface implementation for redisColumn.
func (c redisColumn) IsSortOption() bool  { return c.sortOpt }
func (c redisColumn) SortLabel() string   { return c.sortLbl }
func (c redisColumn) IsDefaultSort() bool { return c.defaultSort }
func (c redisColumn) ColumnName() string  { return c.Name }
func (c redisColumn) SortColumn() string  { return "" }

func redisColumnSet(cols []redisColumn) funcapi.ColumnSet[redisColumn] {
	return funcapi.Columns(cols, func(c redisColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var redisAllColumns = []redisColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "id", Tooltip: "ID", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, UniqueKey: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount}, sortOpt: true, sortLbl: "Top queries by ID"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "timestamp", Tooltip: "Timestamp", Type: funcapi.FieldTypeTimestamp, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, sortOpt: true, sortLbl: "Top queries by Timestamp"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "command", Tooltip: "Command", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "command_name", Tooltip: "Command Name", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, sortOpt: true, sortLbl: "Top queries by Command Name"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "duration", Tooltip: "Duration", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Duration", Title: "Execution Time", IsDefault: true}}, sortOpt: true, sortLbl: "Top queries by Duration", defaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "client_addr", Tooltip: "Client Address", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "client_name", Tooltip: "Client Name", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}},
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// funcTopQueries handles the "top-queries" function for Redis.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topQueriesMethodID {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return []funcapi.ParamConfig{funcapi.BuildSortParam(redisAllColumns)}, nil
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topQueriesMethodID {
		return funcapi.NotFoundResponse(method)
	}

	if f.router.collector.rdb == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	return f.collectTopQueries(ctx, params.Column("__sort"))
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) collectTopQueries(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	c := f.router.collector

	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	entries, err := c.rdb.SlowLogGet(ctx, -1).Result()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &funcapi.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("slowlog query failed: %v", err)}
	}

	cs := redisColumnSet(redisAllColumns)
	sortParam := funcapi.BuildSortParam(redisAllColumns)

	if len(entries) == 0 {
		return &funcapi.FunctionResponse{
			Status:            200,
			Message:           "No slow commands found. SLOWLOG may be empty or disabled.",
			Help:              "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII).",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: "duration",
			RequiredParams:    []funcapi.ParamConfig{sortParam},
			ChartingConfig:    cs.BuildCharting(),
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
			switch col.Name {
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

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "duration",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		ChartingConfig:    cs.BuildCharting(),
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
