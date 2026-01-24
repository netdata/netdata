// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const rethinkMaxQueryTextLength = 4096

type rethinkColumn struct {
	funcapi.ColumnMeta
	IsSortOption  bool   // whether this column appears as a sort option
	SortLabel     string // label for sort option dropdown
	IsDefaultSort bool   // default sort column
}

func rethinkColumnSet(cols []rethinkColumn) funcapi.ColumnSet[rethinkColumn] {
	return funcapi.Columns(cols, func(c rethinkColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var rethinkRunningColumns = []rethinkColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "jobId", Tooltip: "Job ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "durationMs", Tooltip: "Duration", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, IsSortOption: true, IsDefaultSort: true, SortLabel: "Running queries by Duration"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "type", Tooltip: "Type", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientAddress", Tooltip: "Client Address", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientPort", Tooltip: "Client Port", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "servers", Tooltip: "Servers", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}},
}

func rethinkdbMethods() []module.MethodConfig {
	var sortOptions []funcapi.ParamOption
	for _, col := range rethinkRunningColumns {
		if !col.IsSortOption {
			continue
		}
		sortDir := funcapi.FieldSortDescending
		sortOptions = append(sortOptions, funcapi.ParamOption{
			ID:      col.Name,
			Column:  col.Name,
			Name:    col.SortLabel,
			Sort:    &sortDir,
			Default: col.IsDefaultSort,
		})
	}
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running queries from rethinkdb.jobs. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{{
				ID:         "__sort",
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    sortOptions,
				UniqueView: true,
			}},
		},
	}
}

func rethinkdbMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "running-queries":
		var sortOptions []funcapi.ParamOption
		for _, col := range rethinkRunningColumns {
			if !col.IsSortOption {
				continue
			}
			sortDir := funcapi.FieldSortDescending
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.Name,
				Name:    col.SortLabel,
				Sort:    &sortDir,
				Default: col.IsDefaultSort,
			})
		}
		return []funcapi.ParamConfig{{
			ID:         "__sort",
			Name:       "Filter By",
			Help:       "Select the primary sort column",
			Selection:  funcapi.ParamSelect,
			Options:    sortOptions,
			UniqueView: true,
		}}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func rethinkdbHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	if collector.rdb == nil {
		conn, err := collector.newConn(collector.Config)
		if err != nil {
			return &module.FunctionResponse{Status: 503, Message: "collector is still initializing, please retry in a few seconds"}
		}
		collector.rdb = conn
	}

	switch method {
	case "running-queries":
		return collector.collectRunningQueries(ctx, params.Column("__sort"))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

func (c *Collector) collectRunningQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	if ctx.Err() == context.DeadlineExceeded {
		return &module.FunctionResponse{Status: 504, Message: "query timed out"}
	}

	rows, err := c.rdb.jobs(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("jobs query failed: %v", err)}
	}

	jobRows := make([]rethinkJobRow, 0, len(rows))
	for _, row := range rows {
		jobRows = append(jobRows, parseRethinkJob(row))
	}

	cs := rethinkColumnSet(rethinkRunningColumns)
	var sortOptions []funcapi.ParamOption
	for _, col := range rethinkRunningColumns {
		if !col.IsSortOption {
			continue
		}
		sortDir := funcapi.FieldSortDescending
		sortOptions = append(sortOptions, funcapi.ParamOption{
			ID:      col.Name,
			Column:  col.Name,
			Name:    col.SortLabel,
			Sort:    &sortDir,
			Default: col.IsDefaultSort,
		})
	}

	if len(jobRows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running queries found.",
			Help:              "Currently running queries from rethinkdb.jobs",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: mapRethinkSortColumn(sortColumn),
			RequiredParams: []funcapi.ParamConfig{{
				ID:         "__sort",
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    sortOptions,
				UniqueView: true,
			}},
		}
	}

	sortColumn = mapRethinkSortColumn(sortColumn)
	sortRethinkRows(jobRows, sortColumn)
	if len(jobRows) > limit {
		jobRows = jobRows[:limit]
	}

	data := make([][]any, 0, len(jobRows))
	for _, row := range jobRows {
		out := make([]any, len(rethinkRunningColumns))
		for i, col := range rethinkRunningColumns {
			switch col.Name {
			case "jobId":
				out[i] = row.JobID
			case "query":
				out[i] = strmutil.TruncateText(row.Query, rethinkMaxQueryTextLength)
			case "durationMs":
				out[i] = row.DurationMs
			case "type":
				out[i] = row.Type
			case "user":
				out[i] = row.User
			case "clientAddress":
				out[i] = row.ClientAddress
			case "clientPort":
				out[i] = row.ClientPort
			case "servers":
				out[i] = row.Servers
			default:
				out[i] = nil
			}
		}
		data = append(data, out)
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Currently running queries from rethinkdb.jobs. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams: []funcapi.ParamConfig{{
			ID:         "__sort",
			Name:       "Filter By",
			Help:       "Select the primary sort column",
			Selection:  funcapi.ParamSelect,
			Options:    sortOptions,
			UniqueView: true,
		}},
	}
}

type rethinkJobRow struct {
	JobID         string
	Query         string
	DurationMs    float64
	Type          string
	User          string
	ClientAddress string
	ClientPort    int64
	Servers       string
}

func parseRethinkJob(row map[string]any) rethinkJobRow {
	info := mapStringAny(row["info"])
	query := fmt.Sprint(info["query"])
	user := fmt.Sprint(info["user"])
	clientAddr := fmt.Sprint(info["client_address"])
	clientPort := toInt64(info["client_port"])

	servers := ""
	if list, ok := row["servers"].([]any); ok {
		ss := make([]string, 0, len(list))
		for _, v := range list {
			ss = append(ss, fmt.Sprint(v))
		}
		servers = strings.Join(ss, ",")
	}

	return rethinkJobRow{
		JobID:         fmt.Sprint(row["id"]),
		Query:         query,
		DurationMs:    toFloat64(row["duration_sec"]) * 1000,
		Type:          fmt.Sprint(row["type"]),
		User:          user,
		ClientAddress: clientAddr,
		ClientPort:    clientPort,
		Servers:       servers,
	}
}

func mapStringAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func toFloat64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case uint64:
		return float64(t)
	default:
		return 0
	}
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	default:
		return 0
	}
}

func mapRethinkSortColumn(input string) string {
	for _, col := range rethinkRunningColumns {
		if col.IsSortOption && col.Name == input {
			return col.Name
		}
	}
	for _, col := range rethinkRunningColumns {
		if col.IsDefaultSort {
			return col.Name
		}
	}
	for _, col := range rethinkRunningColumns {
		if col.IsSortOption {
			return col.Name
		}
	}
	return ""
}

func sortRethinkRows(rows []rethinkJobRow, sortColumn string) {
	switch sortColumn {
	case "durationMs":
		sort.Slice(rows, func(i, j int) bool { return rows[i].DurationMs > rows[j].DurationMs })
	default:
		sort.Slice(rows, func(i, j int) bool { return rows[i].DurationMs > rows[j].DurationMs })
	}
}
