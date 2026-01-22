// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const couchbaseMaxQueryTextLength = 4096

const (
	paramSort = "__sort"

	ftString    = funcapi.FieldTypeString
	ftInteger   = funcapi.FieldTypeInteger
	ftDuration  = funcapi.FieldTypeDuration
	ftTimestamp = funcapi.FieldTypeTimestamp

	trNone     = funcapi.FieldTransformNone
	trNumber   = funcapi.FieldTransformNumber
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

type couchbaseColumnMeta struct {
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

var couchbaseAllColumns = []couchbaseColumnMeta{
	{id: "requestId", name: "Request ID", colType: ftString, visible: false, sortable: true, filter: filterMulti, visualization: visValue, transform: trText, uniqueKey: true, sortDir: sortDesc, summary: summaryCount},
	{id: "requestTime", name: "Request Time", colType: ftTimestamp, visible: true, sortable: true, filter: filterRange, visualization: visValue, transform: trDatetime, sortDir: sortDesc, summary: summaryMax},
	{id: "statement", name: "Statement", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, sticky: true, fullWidth: true, wrap: true},
	{id: "elapsedTime", name: "Elapsed Time", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "Time", chartTitle: "Elapsed & Service Time", isDefaultChart: true},
	{id: "serviceTime", name: "Service Time", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, transform: trDuration, units: "milliseconds", decimalPoints: 2, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "Time", chartTitle: "Elapsed & Service Time"},
	{id: "resultCount", name: "Result Count", colType: ftInteger, visible: true, sortable: true, filter: filterRange, visualization: visValue, transform: trNumber, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "Results", chartTitle: "Results"},
	{id: "resultSize", name: "Result Size", colType: ftInteger, visible: false, sortable: false, filter: filterRange, visualization: visValue, transform: trNumber, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "ResultSize", chartTitle: "Result Size"},
	{id: "errorCount", name: "Error Count", colType: ftInteger, visible: false, sortable: false, filter: filterRange, visualization: visValue, transform: trNumber, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "Errors", chartTitle: "Errors & Warnings"},
	{id: "warningCount", name: "Warning Count", colType: ftInteger, visible: false, sortable: false, filter: filterRange, visualization: visValue, transform: trNumber, sortDir: sortDesc, summary: summarySum, isMetric: true, chartGroup: "Errors", chartTitle: "Errors & Warnings"},
	{id: "user", name: "User", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true, isPrimary: true},
	{id: "clientContextID", name: "Client Context ID", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},
}

type cbQueryResponse struct {
	Status  string               `json:"status"`
	Results []cbCompletedRequest `json:"results"`
	Errors  []cbQueryError       `json:"errors"`
}

type cbQueryError struct {
	Message string `json:"msg"`
}

type cbCompletedRequest struct {
	RequestID       string      `json:"requestId"`
	RequestTime     string      `json:"requestTime"`
	Statement       string      `json:"statement"`
	ElapsedTime     string      `json:"elapsedTime"`
	ServiceTime     string      `json:"serviceTime"`
	ResultCount     json.Number `json:"resultCount"`
	ResultSize      json.Number `json:"resultSize"`
	ErrorCount      json.Number `json:"errorCount"`
	WarningCount    json.Number `json:"warningCount"`
	User            string      `json:"user"`
	ClientContextID string      `json:"clientContextID"`
}

type cbRow struct {
	RequestID       string
	RequestTime     time.Time
	RequestTimeRaw  string
	Statement       string
	ElapsedMs       float64
	ServiceMs       float64
	ResultCount     int64
	ResultSize      int64
	ErrorCount      int64
	WarningCount    int64
	User            string
	ClientContextID string
}

func couchbaseMethods() []module.MethodConfig {
	sortOptions := buildCouchbaseSortOptions(couchbaseAllColumns)
	return []module.MethodConfig{{
		ID:   "top-queries",
		Name: "Top Queries",
		Help: "Top N1QL requests from system:completed_requests",
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

func couchbaseMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return []funcapi.ParamConfig{buildCouchbaseSortParam(couchbaseAllColumns)}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func couchbaseHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
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

func buildCouchbaseSortOptions(cols []couchbaseColumnMeta) []funcapi.ParamOption {
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
		if col.id == "elapsedTime" {
			opt.Default = true
		}
		sortOptions = append(sortOptions, opt)
	}
	return sortOptions
}

func buildCouchbaseSortParam(cols []couchbaseColumnMeta) funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    buildCouchbaseSortOptions(cols),
		UniqueView: true,
	}
}

func buildCouchbaseColumns(cols []couchbaseColumnMeta) map[string]any {
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

func (c *Collector) queryServiceURL() (string, error) {
	if c.QueryURL != "" {
		return c.QueryURL, nil
	}
	parsed, err := url.Parse(c.URL)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" || port == "8091" {
		port = "8093"
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}
	parsed.Path = ""
	return parsed.String(), nil
}

func (c *Collector) buildQueryRequest(ctx context.Context, statement string) (*http.Request, error) {
	queryURL, err := c.queryServiceURL()
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(queryURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/query/service")

	reqCfg := c.RequestConfig
	reqCfg.URL = u.String()
	reqCfg.Method = http.MethodPost
	reqCfg.Body = url.Values{"statement": {statement}}.Encode()
	if reqCfg.Headers == nil {
		reqCfg.Headers = map[string]string{}
	}
	reqCfg.Headers["Content-Type"] = "application/x-www-form-urlencoded"

	req, err := web.NewHTTPRequest(reqCfg)
	if err != nil {
		return nil, err
	}
	return req.WithContext(ctx), nil
}

func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	statement := "SELECT cr.requestId, cr.requestTime, cr.statement, cr.elapsedTime, cr.serviceTime, " +
		"cr.resultCount, cr.resultSize, cr.errorCount, cr.warningCount, cr.users AS `user`, cr.clientContextID " +
		"FROM system:completed_requests AS cr"

	req, err := c.buildQueryRequest(ctx, statement)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	var resp cbQueryResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}

	if strings.ToLower(resp.Status) != "success" {
		msg := "query failed"
		if len(resp.Errors) > 0 && resp.Errors[0].Message != "" {
			msg = resp.Errors[0].Message
		}
		return &module.FunctionResponse{Status: 500, Message: msg}
	}

	rows := make([]cbRow, 0, len(resp.Results))
	for _, r := range resp.Results {
		rows = append(rows, buildCouchbaseRow(r))
	}

	if len(rows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No completed requests found.",
			Help:              "Top N1QL requests from system:completed_requests",
			Columns:           buildCouchbaseColumns(couchbaseAllColumns),
			Data:              [][]any{},
			DefaultSortColumn: "elapsedTime",
			RequiredParams:    []funcapi.ParamConfig{buildCouchbaseSortParam(couchbaseAllColumns)},
			Charts:            couchbaseTopQueriesCharts(couchbaseAllColumns),
			DefaultCharts:     couchbaseTopQueriesDefaultCharts(couchbaseAllColumns),
			GroupBy:           couchbaseTopQueriesGroupBy(couchbaseAllColumns),
		}
	}

	sortColumn = mapCouchbaseSortColumn(sortColumn)
	sortCouchbaseRows(rows, sortColumn)

	if len(rows) > limit {
		rows = rows[:limit]
	}

	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		out := make([]any, len(couchbaseAllColumns))
		for i, col := range couchbaseAllColumns {
			switch col.id {
			case "requestId":
				out[i] = row.RequestID
			case "requestTime":
				if row.RequestTime.IsZero() {
					out[i] = row.RequestTimeRaw
				} else {
					out[i] = row.RequestTime.Format(time.RFC3339Nano)
				}
			case "statement":
				out[i] = strmutil.TruncateText(row.Statement, couchbaseMaxQueryTextLength)
			case "elapsedTime":
				out[i] = row.ElapsedMs
			case "serviceTime":
				out[i] = row.ServiceMs
			case "resultCount":
				out[i] = row.ResultCount
			case "resultSize":
				out[i] = row.ResultSize
			case "errorCount":
				out[i] = row.ErrorCount
			case "warningCount":
				out[i] = row.WarningCount
			case "user":
				out[i] = row.User
			case "clientContextID":
				out[i] = row.ClientContextID
			default:
				out[i] = nil
			}
		}
		data = append(data, out)
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top N1QL requests from system:completed_requests",
		Columns:           buildCouchbaseColumns(couchbaseAllColumns),
		Data:              data,
		DefaultSortColumn: "elapsedTime",
		RequiredParams:    []funcapi.ParamConfig{buildCouchbaseSortParam(couchbaseAllColumns)},
		Charts:            couchbaseTopQueriesCharts(couchbaseAllColumns),
		DefaultCharts:     couchbaseTopQueriesDefaultCharts(couchbaseAllColumns),
		GroupBy:           couchbaseTopQueriesGroupBy(couchbaseAllColumns),
	}
}

func buildCouchbaseRow(r cbCompletedRequest) cbRow {
	row := cbRow{
		RequestID:       r.RequestID,
		RequestTimeRaw:  r.RequestTime,
		Statement:       r.Statement,
		User:            r.User,
		ClientContextID: r.ClientContextID,
	}

	if t, err := time.Parse(time.RFC3339Nano, r.RequestTime); err == nil {
		row.RequestTime = t
	} else if t, err := time.Parse(time.RFC3339, r.RequestTime); err == nil {
		row.RequestTime = t
	}

	row.ElapsedMs = parseDurationMs(r.ElapsedTime)
	row.ServiceMs = parseDurationMs(r.ServiceTime)
	row.ResultCount = parseNumber(r.ResultCount)
	row.ResultSize = parseNumber(r.ResultSize)
	row.ErrorCount = parseNumber(r.ErrorCount)
	row.WarningCount = parseNumber(r.WarningCount)

	return row
}

func parseDurationMs(raw string) float64 {
	if raw == "" {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return float64(d) / float64(time.Millisecond)
	}
	return 0
}

func parseNumber(n json.Number) int64 {
	if n == "" {
		return 0
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return int64(f)
	}
	return 0
}

func mapCouchbaseSortColumn(col string) string {
	switch col {
	case "elapsedTime", "serviceTime", "requestTime", "resultCount":
		return col
	default:
		return "elapsedTime"
	}
}

func sortCouchbaseRows(rows []cbRow, sortColumn string) {
	switch sortColumn {
	case "serviceTime":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].ServiceMs > rows[j].ServiceMs
		})
	case "requestTime":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].RequestTime.After(rows[j].RequestTime)
		})
	case "resultCount":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].ResultCount > rows[j].ResultCount
		})
	default:
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].ElapsedMs > rows[j].ElapsedMs
		})
	}
}

func couchbaseTopQueriesCharts(cols []couchbaseColumnMeta) map[string]module.ChartConfig {
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

func couchbaseTopQueriesDefaultCharts(cols []couchbaseColumnMeta) [][]string {
	label := primaryCouchbaseLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultCouchbaseChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func couchbaseTopQueriesGroupBy(cols []couchbaseColumnMeta) map[string]module.GroupByConfig {
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

func primaryCouchbaseLabel(cols []couchbaseColumnMeta) string {
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

func defaultCouchbaseChartGroups(cols []couchbaseColumnMeta) []string {
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
