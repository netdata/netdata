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
	summaryMax   = funcapi.FieldSummaryMax

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

type rethinkColumnMeta struct {
	id            string
	name          string
	colType       funcapi.FieldType
	visible       bool
	sortable      bool
	fullWidth     bool
	wrap          bool
	sticky        bool
	filter        funcapi.FieldFilter
	visualization funcapi.FieldVisual
	transform     funcapi.FieldTransform
	units         string
	decimalPoints int
	uniqueKey     bool
	sortDir       funcapi.FieldSort
	summary       funcapi.FieldSummary
	sortLabel     string
	isSortOption  bool
	isDefaultSort bool
}

var rethinkRunningColumns = []rethinkColumnMeta{
	{id: "jobId", name: "Job ID", colType: ftString, visible: false, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, uniqueKey: true, sortDir: sortAsc, summary: summaryCount},
	{id: "query", name: "Query", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "durationMs", name: "Duration", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, isSortOption: true, isDefaultSort: true, sortLabel: "Running queries by Duration"},
	{id: "type", name: "Type", colType: ftString, visible: true, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "user", name: "User", colType: ftString, visible: true, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "clientAddress", name: "Client Address", colType: ftString, visible: false, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, sortDir: sortAsc, summary: summaryCount},
	{id: "clientPort", name: "Client Port", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, transform: trNumber, sortDir: sortDesc, summary: summaryMax},
	{id: "servers", name: "Servers", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},
}

func rethinkdbMethods() []module.MethodConfig {
	sortOptions := buildRethinkSortOptions(rethinkRunningColumns)
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running queries from rethinkdb.jobs. WARNING: Query text may contain unmasked literals (potential PII).",
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

func rethinkdbMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "running-queries":
		return []funcapi.ParamConfig{buildRethinkSortParam(rethinkRunningColumns)}, nil
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
		return collector.collectRunningQueries(ctx, params.Column(paramSort))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

func buildRethinkSortOptions(cols []rethinkColumnMeta) []funcapi.ParamOption {
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

func buildRethinkSortParam(cols []rethinkColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildRethinkSortOptions(cols),
		UniqueView: true,
	}
}

func buildRethinkColumns(cols []rethinkColumnMeta) map[string]any {
	result := make(map[string]any, len(cols))
	for i, col := range cols {
		visual := visValue
		if col.colType == ftDuration {
			visual = visBar
		}
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.name,
			Type:                  col.colType,
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

	if len(jobRows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No running queries found.",
			Help:              "Currently running queries from rethinkdb.jobs",
			Columns:           buildRethinkColumns(rethinkRunningColumns),
			Data:              [][]any{},
			DefaultSortColumn: mapRethinkSortColumn(sortColumn),
			RequiredParams:    []funcapi.ParamConfig{buildRethinkSortParam(rethinkRunningColumns)},
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
			switch col.id {
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
		Columns:           buildRethinkColumns(rethinkRunningColumns),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{buildRethinkSortParam(rethinkRunningColumns)},
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
		if col.isSortOption && col.id == input {
			return col.id
		}
	}
	for _, col := range rethinkRunningColumns {
		if col.isDefaultSort {
			return col.id
		}
	}
	for _, col := range rethinkRunningColumns {
		if col.isSortOption {
			return col.id
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
