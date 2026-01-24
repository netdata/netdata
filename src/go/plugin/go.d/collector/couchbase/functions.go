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

type cbColumn struct {
	funcapi.ColumnMeta
	IsSortOption  bool   // whether this column appears as a sort option
	SortLabel     string // label for sort option dropdown
	IsDefaultSort bool   // default sort column
}

func cbColumnSet(cols []cbColumn) funcapi.ColumnSet[cbColumn] {
	return funcapi.Columns(cols, func(c cbColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var couchbaseAllColumns = []cbColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "requestId", Tooltip: "Request ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, UniqueKey: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryCount}, IsSortOption: true, SortLabel: "Top queries by Request ID"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "requestTime", Tooltip: "Request Time", Type: funcapi.FieldTypeTimestamp, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, IsSortOption: true, SortLabel: "Top queries by Request Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "statement", Tooltip: "Statement", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, Sticky: true, FullWidth: true, Wrap: true}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "elapsedTime", Tooltip: "Elapsed Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Elapsed & Service Time", IsDefault: true}}, IsSortOption: true, IsDefaultSort: true, SortLabel: "Top queries by Elapsed Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "serviceTime", Tooltip: "Service Time", Type: funcapi.FieldTypeDuration, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformDuration, Units: "milliseconds", DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Elapsed & Service Time"}}, IsSortOption: true, SortLabel: "Top queries by Service Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "resultCount", Tooltip: "Result Count", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Results", Title: "Results"}}, IsSortOption: true, SortLabel: "Top queries by Result Count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "resultSize", Tooltip: "Result Size", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "ResultSize", Title: "Result Size"}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "errorCount", Tooltip: "Error Count", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Errors", Title: "Errors & Warnings"}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "warningCount", Tooltip: "Warning Count", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Chart: &funcapi.ChartOptions{Group: "Errors", Title: "Errors & Warnings"}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientContextID", Tooltip: "Client Context ID", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}},
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
	var sortOptions []funcapi.ParamOption
	for _, col := range couchbaseAllColumns {
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
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top N1QL requests from system:completed_requests",
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

func couchbaseMethodParams(_ context.Context, _ *module.Job, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		var sortOptions []funcapi.ParamOption
		for _, col := range couchbaseAllColumns {
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
		return collector.collectTopQueries(ctx, params.Column("__sort"))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
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

	cs := cbColumnSet(couchbaseAllColumns)
	var sortOptions []funcapi.ParamOption
	for _, col := range couchbaseAllColumns {
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

	if len(rows) == 0 {
		return &module.FunctionResponse{
			Status:            200,
			Message:           "No completed requests found.",
			Help:              "Top N1QL requests from system:completed_requests",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: "elapsedTime",
			RequiredParams: []funcapi.ParamConfig{{
				ID:         "__sort",
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    sortOptions,
				UniqueView: true,
			}},
			Charts:        cs.BuildCharts(),
			DefaultCharts: cs.BuildDefaultCharts(),
			GroupBy:       cs.BuildGroupBy(),
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
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "elapsedTime",
		RequiredParams: []funcapi.ParamConfig{{
			ID:         "__sort",
			Name:       "Filter By",
			Help:       "Select the primary sort column",
			Selection:  funcapi.ParamSelect,
			Options:    sortOptions,
			UniqueView: true,
		}},
		Charts:        cs.BuildCharts(),
		DefaultCharts: cs.BuildDefaultCharts(),
		GroupBy:       cs.BuildGroupBy(),
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
