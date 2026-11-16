// SPDX-License-Identifier: GPL-3.0-or-later

package redisfunc

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	topQueriesMethodID         = "top-queries"
	topQueriesHelp             = "Slow commands from Redis SLOWLOG. WARNING: Command arguments may contain unmasked literals (potential PII)."
	topQueriesDefaultSort      = "duration"
	topQueriesMaxCommandLength = 4096
)

func topQueriesFunctionConfig() funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           topQueriesHelp,
		RequireCloud:   true,
		RequiredParams: topQueriesParams(),
	}
}

func topQueriesParams() []funcapi.ParamConfig {
	return []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)}
}

type topQueriesColumn struct {
	funcapi.ColumnMeta
	sortOpt     bool   // whether this column appears as a sort option
	sortLbl     string // label for sort option dropdown
	defaultSort bool   // default sort column
}

// funcapi.SortableColumn interface implementation for topQueriesColumn.
func (c topQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c topQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c topQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c topQueriesColumn) ColumnName() string  { return c.Name }
func (c topQueriesColumn) SortColumn() string  { return "" }

func topQueriesColumnSet() funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(topQueriesColumns, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var topQueriesColumns = []topQueriesColumn{
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "id",
			Tooltip:       "ID",
			Type:          funcapi.FieldTypeInteger,
			Visible:       false,
			Sortable:      true,
			Filter:        funcapi.FieldFilterRange,
			Visualization: funcapi.FieldVisualValue,
			Transform:     funcapi.FieldTransformNumber,
			UniqueKey:     true,
			Sort:          funcapi.FieldSortDescending,
			Summary:       funcapi.FieldSummaryCount,
		},
		sortOpt: true,
		sortLbl: "Top queries by ID",
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "timestamp",
			Tooltip:       "Timestamp",
			Type:          funcapi.FieldTypeTimestamp,
			Visible:       true,
			Sortable:      true,
			Filter:        funcapi.FieldFilterRange,
			Visualization: funcapi.FieldVisualValue,
			Transform:     funcapi.FieldTransformDatetime,
			Sort:          funcapi.FieldSortDescending,
			Summary:       funcapi.FieldSummaryMax,
		},
		sortOpt: true,
		sortLbl: "Top queries by Timestamp",
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "command",
			Tooltip:       "Command",
			Type:          funcapi.FieldTypeString,
			Visible:       true,
			Sortable:      false,
			Filter:        funcapi.FieldFilterMultiselect,
			Visualization: funcapi.FieldVisualValue,
			Transform:     funcapi.FieldTransformText,
			Sticky:        true,
			FullWidth:     true,
			Wrap:          true,
		},
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "command_name",
			Tooltip:       "Command Name",
			Type:          funcapi.FieldTypeString,
			Visible:       true,
			Sortable:      true,
			Filter:        funcapi.FieldFilterMultiselect,
			Visualization: funcapi.FieldVisualValue,
			Transform:     funcapi.FieldTransformText,
			Sort:          funcapi.FieldSortAscending,
			GroupBy: &funcapi.GroupByOptions{
				IsDefault: true,
			},
		},
		sortOpt: true,
		sortLbl: "Top queries by Command Name",
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "duration",
			Tooltip:       "Duration",
			Type:          funcapi.FieldTypeDuration,
			Visible:       true,
			Sortable:      true,
			Filter:        funcapi.FieldFilterRange,
			Visualization: funcapi.FieldVisualBar,
			Transform:     funcapi.FieldTransformDuration,
			Units:         "milliseconds",
			DecimalPoints: 2,
			Sort:          funcapi.FieldSortDescending,
			Summary:       funcapi.FieldSummarySum,
			Chart: &funcapi.ChartOptions{
				Group:     "Duration",
				Title:     "Execution Time",
				IsDefault: true,
			},
		},
		sortOpt:     true,
		sortLbl:     "Top queries by Duration",
		defaultSort: true,
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "client_addr",
			Tooltip:       "Client Address",
			Type:          funcapi.FieldTypeString,
			Visible:       false,
			Sortable:      false,
			Filter:        funcapi.FieldFilterMultiselect,
			Visualization: funcapi.FieldVisualValue,
			Transform:     funcapi.FieldTransformText,
			GroupBy:       &funcapi.GroupByOptions{},
		},
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "client_name",
			Tooltip:       "Client Name",
			Type:          funcapi.FieldTypeString,
			Visible:       false,
			Sortable:      false,
			Filter:        funcapi.FieldFilterMultiselect,
			Visualization: funcapi.FieldVisualValue,
			Transform:     funcapi.FieldTransformText,
			GroupBy:       &funcapi.GroupByOptions{},
		},
	},
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// funcTopQueries serves the top-queries Function from the server SLOWLOG.
type funcTopQueries struct {
	router *router
}

func newFuncTopQueries(r *router) *funcTopQueries {
	return &funcTopQueries{
		router: r,
	}
}

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	if f.router.cfg.TopQueries.Disabled {
		return nil, errors.New("top-queries function disabled in configuration")
	}
	return topQueriesParams(), nil
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(
	ctx context.Context,
	_ string,
	params funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	cfg := f.router.cfg
	if cfg.TopQueries.Disabled {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}

	client, err := f.router.deps.Client()
	if err != nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.topQueriesTimeout())
	defer cancel()

	entries, err := client.SlowLogGet(ctx, -1).Result()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("slowlog query failed: %v", err)
	}

	sortSlowLogEntries(entries, params.Column("__sort"))
	entries = entries[:min(len(entries), cfg.topQueriesLimit())]

	data := make([][]any, 0, len(entries))
	for _, entry := range entries {
		data = append(data, topQueriesRow(entry))
	}

	cs := topQueriesColumnSet()
	resp := &funcapi.FunctionResponse{
		Status:            200,
		Help:              topQueriesHelp,
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: topQueriesDefaultSort,
		RequiredParams:    topQueriesParams(),
		ChartingConfig:    cs.BuildCharting(),
	}
	if len(entries) == 0 {
		resp.Message = "No slow commands found. SLOWLOG may be empty or disabled."
	}
	return resp
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(context.Context) {}

func topQueriesRow(entry redis.SlowLog) []any {
	row := make([]any, len(topQueriesColumns))
	for i, col := range topQueriesColumns {
		switch col.Name {
		case "id":
			row[i] = entry.ID
		case "timestamp":
			row[i] = entry.Time.Format(time.RFC3339Nano)
		case "command":
			row[i] = strmutil.TruncateText(strings.Join(entry.Args, " "), topQueriesMaxCommandLength)
		case "command_name":
			row[i] = slowLogCommandName(entry)
		case "duration":
			row[i] = float64(entry.Duration) / float64(time.Millisecond)
		case "client_addr":
			row[i] = entry.ClientAddr
		case "client_name":
			row[i] = entry.ClientName
		}
	}
	return row
}

// sortSlowLogEntries orders entries by a sort option ID; any other value sorts by duration.
func sortSlowLogEntries(entries []redis.SlowLog, column string) {
	switch column {
	case "timestamp":
		slices.SortFunc(entries, func(a, b redis.SlowLog) int { return b.Time.Compare(a.Time) })
	case "id":
		slices.SortFunc(entries, func(a, b redis.SlowLog) int { return cmp.Compare(b.ID, a.ID) })
	case "command_name":
		slices.SortFunc(entries, func(a, b redis.SlowLog) int {
			return cmp.Compare(slowLogCommandName(a), slowLogCommandName(b))
		})
	default:
		slices.SortFunc(entries, func(a, b redis.SlowLog) int { return cmp.Compare(b.Duration, a.Duration) })
	}
}

func slowLogCommandName(entry redis.SlowLog) string {
	if len(entry.Args) == 0 {
		return ""
	}
	return entry.Args[0]
}
