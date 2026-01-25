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

const (
	topQueriesMethodID      = "top-queries"
	topQueriesMaxTextLength = 4096
)

func topQueriesMethodConfig() module.MethodConfig {
	return module.MethodConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Top N1QL requests from system:completed_requests",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)},
	}
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

var topQueriesColumns = []topQueriesColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "requestId", Tooltip: "Request ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount}, sortOpt: true, sortLbl: "Top queries by Request ID"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "requestTime", Tooltip: "Request Time", Type: funcapi.FieldTypeTimestamp, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, sortOpt: true, sortLbl: "Top queries by Request Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "statement", Tooltip: "Statement", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "elapsedTime", Tooltip: "Elapsed Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Elapsed & Service Time", IsDefault: true}}, sortOpt: true, defaultSort: true, sortLbl: "Top queries by Elapsed Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "serviceTime", Tooltip: "Service Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Elapsed & Service Time"}}, sortOpt: true, sortLbl: "Top queries by Service Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "resultCount", Tooltip: "Result Count", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Results", Title: "Results"}}, sortOpt: true, sortLbl: "Top queries by Result Count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "resultSize", Tooltip: "Result Size", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "ResultSize", Title: "Result Size"}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "errorCount", Tooltip: "Error Count", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Errors", Title: "Errors & Warnings"}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "warningCount", Tooltip: "Warning Count", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Errors", Title: "Errors & Warnings"}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientContextID", Tooltip: "Client Context ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}},
}

type topQueriesResponse struct {
	Status  string                  `json:"status"`
	Results []topQueriesRequestData `json:"results"`
	Errors  []topQueriesError       `json:"errors"`
}

type topQueriesError struct {
	Message string `json:"msg"`
}

type topQueriesRequestData struct {
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

type topQueriesRow struct {
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

// funcTopQueries implements funcapi.MethodHandler for Couchbase top-queries.
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
	case topQueriesMethodID:
		return []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)}, nil
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
	case topQueriesMethodID:
		return f.collectData(ctx, params.Column("__sort"))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) collectData(ctx context.Context, sortColumn string) *module.FunctionResponse {
	limit := f.router.collector.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	statement := "SELECT cr.requestId, cr.requestTime, cr.statement, cr.elapsedTime, cr.serviceTime, " +
		"cr.resultCount, cr.resultSize, cr.errorCount, cr.warningCount, cr.users AS `user`, cr.clientContextID " +
		"FROM system:completed_requests AS cr"

	req, err := f.buildQueryRequest(ctx, statement)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	var resp topQueriesResponse
	if err := web.DoHTTP(f.router.collector.httpClient).RequestJSON(req, &resp); err != nil {
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

	rows := make([]topQueriesRow, 0, len(resp.Results))
	for _, r := range resp.Results {
		rows = append(rows, f.buildRow(r))
	}

	cs := f.columnSet(topQueriesColumns)
	sortParam := funcapi.BuildSortParam(topQueriesColumns)

	if len(rows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No completed requests found.",
			Help:              "Top N1QL requests from system:completed_requests",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: "elapsedTime",
			RequiredParams:    []funcapi.ParamConfig{sortParam},
			ChartingConfig:    cs.BuildCharting(),
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
			case "requestId":
				out[i] = row.RequestID
			case "requestTime":
				if row.RequestTime.IsZero() {
					out[i] = row.RequestTimeRaw
				} else {
					out[i] = row.RequestTime.Format(time.RFC3339Nano)
				}
			case "statement":
				out[i] = strmutil.TruncateText(row.Statement, topQueriesMaxTextLength)
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
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "elapsedTime",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		ChartingConfig:    cs.BuildCharting(),
	}
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) queryServiceURL() (string, error) {
	if f.router.collector.QueryURL != "" {
		return f.router.collector.QueryURL, nil
	}
	parsed, err := url.Parse(f.router.collector.URL)
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

func (f *funcTopQueries) buildQueryRequest(ctx context.Context, statement string) (*http.Request, error) {
	queryURL, err := f.queryServiceURL()
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(queryURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/query/service")

	reqCfg := f.router.collector.RequestConfig
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

func (f *funcTopQueries) buildRow(r topQueriesRequestData) topQueriesRow {
	row := topQueriesRow{
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

	row.ElapsedMs = f.parseDurationMs(r.ElapsedTime)
	row.ServiceMs = f.parseDurationMs(r.ServiceTime)
	row.ResultCount = f.parseNumber(r.ResultCount)
	row.ResultSize = f.parseNumber(r.ResultSize)
	row.ErrorCount = f.parseNumber(r.ErrorCount)
	row.WarningCount = f.parseNumber(r.WarningCount)

	return row
}

func (f *funcTopQueries) parseDurationMs(raw string) float64 {
	if raw == "" {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return float64(d) / float64(time.Millisecond)
	}
	return 0
}

func (f *funcTopQueries) parseNumber(n json.Number) int64 {
	if n == "" {
		return 0
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if fv, err := n.Float64(); err == nil {
		return int64(fv)
	}
	return 0
}

func (f *funcTopQueries) mapSortColumn(col string) string {
	switch col {
	case "elapsedTime", "serviceTime", "requestTime", "resultCount":
		return col
	default:
		return "elapsedTime"
	}
}

func (f *funcTopQueries) sortRows(rows []topQueriesRow, sortColumn string) {
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
