// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type (
	Charts = module.Charts
	Chart  = module.Chart
	Dims   = module.Dims
	Dim    = module.Dim
)

const (
	prioReqTotal = module.Priority + iota
	prioReqExcluded
	prioReqType

	prioRespCodesClass
	prioRespCodes
	prioRespCodes1xx
	prioRespCodes2xx
	prioRespCodes3xx
	prioRespCodes4xx
	prioRespCodes5xx

	prioBandwidth

	prioReqProcTime
	prioRespTimeHist
	prioUpsRespTime
	prioUpsRespTimeHist

	prioUniqIP

	prioReqVhost
	prioReqPort
	prioReqScheme
	prioReqMethod
	prioReqVersion
	prioReqIPProto
	prioReqSSLProto
	prioReqSSLCipherSuite

	prioReqCustomFieldPattern  // chart per custom field, alphabetical order
	prioReqCustomTimeField     // chart per custom time field, alphabetical order
	prioReqCustomTimeFieldHist // histogram chart per custom time field
	prioReqURLPattern
	prioURLPatternStats

	prioReqCustomNumericFieldSummary // 3 charts per url pattern, alphabetical order
)

// NOTE: inconsistency with python web_log
// TODO: current histogram charts are misleading in netdata

// Requests
var (
	reqTotal = Chart{
		ID:       "requests",
		Title:    "Total Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "web_log.requests",
		Priority: prioReqTotal,
		Dims: Dims{
			{ID: "requests", Algo: module.Incremental},
		},
	}
	reqExcluded = Chart{
		ID:       "excluded_requests",
		Title:    "Excluded Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "web_log.excluded_requests",
		Type:     module.Stacked,
		Priority: prioReqExcluded,
		Dims: Dims{
			{ID: "req_unmatched", Name: "unmatched", Algo: module.Incremental},
		},
	}
	// netdata specific grouping
	reqTypes = Chart{
		ID:       "requests_by_type",
		Title:    "Requests By Type",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "web_log.type_requests",
		Type:     module.Stacked,
		Priority: prioReqType,
		Dims: Dims{
			{ID: "req_type_success", Name: "success", Algo: module.Incremental},
			{ID: "req_type_bad", Name: "bad", Algo: module.Incremental},
			{ID: "req_type_redirect", Name: "redirect", Algo: module.Incremental},
			{ID: "req_type_error", Name: "error", Algo: module.Incremental},
		},
	}
)

// Responses
var (
	respCodeClass = Chart{
		ID:       "responses_by_status_code_class",
		Title:    "Responses By Status Code Class",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_class_responses",
		Type:     module.Stacked,
		Priority: prioRespCodesClass,
		Dims: Dims{
			{ID: "resp_2xx", Name: "2xx", Algo: module.Incremental},
			{ID: "resp_5xx", Name: "5xx", Algo: module.Incremental},
			{ID: "resp_3xx", Name: "3xx", Algo: module.Incremental},
			{ID: "resp_4xx", Name: "4xx", Algo: module.Incremental},
			{ID: "resp_1xx", Name: "1xx", Algo: module.Incremental},
		},
	}
	respCodes = Chart{
		ID:       "responses_by_status_code",
		Title:    "Responses By Status Code",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_responses",
		Type:     module.Stacked,
		Priority: prioRespCodes,
	}
	respCodes1xx = Chart{
		ID:       "status_code_class_1xx_responses",
		Title:    "Informational Responses By Status Code",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_class_1xx_responses",
		Type:     module.Stacked,
		Priority: prioRespCodes1xx,
	}
	respCodes2xx = Chart{
		ID:       "status_code_class_2xx_responses",
		Title:    "Successful Responses By Status Code",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_class_2xx_responses",
		Type:     module.Stacked,
		Priority: prioRespCodes2xx,
	}
	respCodes3xx = Chart{
		ID:       "status_code_class_3xx_responses",
		Title:    "Redirects Responses By Status Code",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_class_3xx_responses",
		Type:     module.Stacked,
		Priority: prioRespCodes3xx,
	}
	respCodes4xx = Chart{
		ID:       "status_code_class_4xx_responses",
		Title:    "Client Errors Responses By Status Code",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_class_4xx_responses",
		Type:     module.Stacked,
		Priority: prioRespCodes4xx,
	}
	respCodes5xx = Chart{
		ID:       "status_code_class_5xx_responses",
		Title:    "Server Errors Responses By Status Code",
		Units:    "responses/s",
		Fam:      "responses",
		Ctx:      "web_log.status_code_class_5xx_responses",
		Type:     module.Stacked,
		Priority: prioRespCodes5xx,
	}
)

// Bandwidth
var (
	bandwidth = Chart{
		ID:       "bandwidth",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "bandwidth",
		Ctx:      "web_log.bandwidth",
		Type:     module.Area,
		Priority: prioBandwidth,
		Dims: Dims{
			{ID: "bytes_received", Name: "received", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	}
)

// Timings
var (
	reqProcTime = Chart{
		ID:       "request_processing_time",
		Title:    "Request Processing Time",
		Units:    "milliseconds",
		Fam:      "timings",
		Ctx:      "web_log.request_processing_time",
		Priority: prioReqProcTime,
		Dims: Dims{
			{ID: "req_proc_time_min", Name: "min", Div: 1000},
			{ID: "req_proc_time_max", Name: "max", Div: 1000},
			{ID: "req_proc_time_avg", Name: "avg", Div: 1000},
		},
	}
	reqProcTimeHist = Chart{
		ID:       "requests_processing_time_histogram",
		Title:    "Requests Processing Time Histogram",
		Units:    "requests/s",
		Fam:      "timings",
		Ctx:      "web_log.requests_processing_time_histogram",
		Priority: prioRespTimeHist,
	}
)

// Upstream
var (
	upsRespTime = Chart{
		ID:       "upstream_response_time",
		Title:    "Upstream Response Time",
		Units:    "milliseconds",
		Fam:      "timings",
		Ctx:      "web_log.upstream_response_time",
		Priority: prioUpsRespTime,
		Dims: Dims{
			{ID: "upstream_resp_time_min", Name: "min", Div: 1000},
			{ID: "upstream_resp_time_max", Name: "max", Div: 1000},
			{ID: "upstream_resp_time_avg", Name: "avg", Div: 1000},
		},
	}
	upsRespTimeHist = Chart{
		ID:       "upstream_responses_time_histogram",
		Title:    "Upstream Responses Time Histogram",
		Units:    "responses/s",
		Fam:      "timings",
		Ctx:      "web_log.upstream_responses_time_histogram",
		Priority: prioUpsRespTimeHist,
	}
)

// Clients
var (
	uniqIPsCurPoll = Chart{
		ID:       "current_poll_uniq_clients",
		Title:    "Current Poll Unique Clients",
		Units:    "clients",
		Fam:      "client",
		Ctx:      "web_log.current_poll_uniq_clients",
		Type:     module.Stacked,
		Priority: prioUniqIP,
		Dims: Dims{
			{ID: "uniq_ipv4", Name: "ipv4", Algo: module.Absolute},
			{ID: "uniq_ipv6", Name: "ipv6", Algo: module.Absolute},
		},
	}
)

// Request By N
var (
	reqByVhost = Chart{
		ID:       "requests_by_vhost",
		Title:    "Requests By Vhost",
		Units:    "requests/s",
		Fam:      "vhost",
		Ctx:      "web_log.vhost_requests",
		Type:     module.Stacked,
		Priority: prioReqVhost,
	}
	reqByPort = Chart{
		ID:       "requests_by_port",
		Title:    "Requests By Port",
		Units:    "requests/s",
		Fam:      "port",
		Ctx:      "web_log.port_requests",
		Type:     module.Stacked,
		Priority: prioReqPort,
	}
	reqByScheme = Chart{
		ID:       "requests_by_scheme",
		Title:    "Requests By Scheme",
		Units:    "requests/s",
		Fam:      "scheme",
		Ctx:      "web_log.scheme_requests",
		Type:     module.Stacked,
		Priority: prioReqScheme,
		Dims: Dims{
			{ID: "req_http_scheme", Name: "http", Algo: module.Incremental},
			{ID: "req_https_scheme", Name: "https", Algo: module.Incremental},
		},
	}
	reqByMethod = Chart{
		ID:       "requests_by_http_method",
		Title:    "Requests By HTTP Method",
		Units:    "requests/s",
		Fam:      "http method",
		Ctx:      "web_log.http_method_requests",
		Type:     module.Stacked,
		Priority: prioReqMethod,
	}
	reqByVersion = Chart{
		ID:       "requests_by_http_version",
		Title:    "Requests By HTTP Version",
		Units:    "requests/s",
		Fam:      "http version",
		Ctx:      "web_log.http_version_requests",
		Type:     module.Stacked,
		Priority: prioReqVersion,
	}
	reqByIPProto = Chart{
		ID:       "requests_by_ip_proto",
		Title:    "Requests By IP Protocol",
		Units:    "requests/s",
		Fam:      "ip proto",
		Ctx:      "web_log.ip_proto_requests",
		Type:     module.Stacked,
		Priority: prioReqIPProto,
		Dims: Dims{
			{ID: "req_ipv4", Name: "ipv4", Algo: module.Incremental},
			{ID: "req_ipv6", Name: "ipv6", Algo: module.Incremental},
		},
	}
	reqBySSLProto = Chart{
		ID:       "requests_by_ssl_proto",
		Title:    "Requests By SSL Connection Protocol",
		Units:    "requests/s",
		Fam:      "ssl conn",
		Ctx:      "web_log.ssl_proto_requests",
		Type:     module.Stacked,
		Priority: prioReqSSLProto,
	}
	reqBySSLCipherSuite = Chart{
		ID:       "requests_by_ssl_cipher_suite",
		Title:    "Requests By SSL Connection Cipher Suite",
		Units:    "requests/s",
		Fam:      "ssl conn",
		Ctx:      "web_log.ssl_cipher_suite_requests",
		Type:     module.Stacked,
		Priority: prioReqSSLCipherSuite,
	}
)

// Request By N Patterns
var (
	reqByURLPattern = Chart{
		ID:       "requests_by_url_pattern",
		Title:    "URL Field Requests By Pattern",
		Units:    "requests/s",
		Fam:      "url ptn",
		Ctx:      "web_log.url_pattern_requests",
		Type:     module.Stacked,
		Priority: prioReqURLPattern,
	}
	reqByCustomFieldPattern = Chart{
		ID:       "custom_field_%s_requests_by_pattern",
		Title:    "Custom Field %s Requests By Pattern",
		Units:    "requests/s",
		Fam:      "custom field ptn",
		Ctx:      "web_log.custom_field_pattern_requests",
		Type:     module.Stacked,
		Priority: prioReqCustomFieldPattern,
	}
)

// custom time field
var (
	reqByCustomTimeField = Chart{
		ID:       "custom_time_field_%s_summary",
		Title:    `Custom Time Field "%s" Summary`,
		Units:    "milliseconds",
		Fam:      "custom time field",
		Ctx:      "web_log.custom_time_field_summary",
		Priority: prioReqCustomTimeField,
		Dims: Dims{
			{ID: "custom_time_field_%s_time_min", Name: "min", Div: 1000},
			{ID: "custom_time_field_%s_time_max", Name: "max", Div: 1000},
			{ID: "custom_time_field_%s_time_avg", Name: "avg", Div: 1000},
		},
	}
	reqByCustomTimeFieldHist = Chart{
		ID:       "custom_time_field_%s_histogram",
		Title:    `Custom Time Field "%s" Histogram`,
		Units:    "observations",
		Fam:      "custom time field",
		Ctx:      "web_log.custom_time_field_histogram",
		Priority: prioReqCustomTimeFieldHist,
	}
)

var (
	customNumericFieldSummaryChartTmpl = Chart{
		ID:       "custom_numeric_field_%s_summary",
		Title:    "Custom Numeric Field Summary",
		Units:    "",
		Fam:      "custom numeric fields",
		Ctx:      "web_log.custom_numeric_field_%s_summary",
		Priority: prioReqCustomNumericFieldSummary,
		Dims: Dims{
			{ID: "custom_numeric_field_%s_summary_min", Name: "min"},
			{ID: "custom_numeric_field_%s_summary_max", Name: "max"},
			{ID: "custom_numeric_field_%s_summary_avg", Name: "avg"},
		},
	}
)

// URL pattern stats
var (
	urlPatternRespCodes = Chart{
		ID:       "url_pattern_%s_responses_by_status_code",
		Title:    "Responses By Status Code",
		Units:    "responses/s",
		Fam:      "url ptn %s",
		Ctx:      "web_log.url_pattern_status_code_responses",
		Type:     module.Stacked,
		Priority: prioURLPatternStats,
	}
	urlPatternReqMethods = Chart{
		ID:       "url_pattern_%s_requests_by_http_method",
		Title:    "Requests By HTTP Method",
		Units:    "requests/s",
		Fam:      "url ptn %s",
		Ctx:      "web_log.url_pattern_http_method_requests",
		Type:     module.Stacked,
		Priority: prioURLPatternStats + 1,
	}
	urlPatternBandwidth = Chart{
		ID:       "url_pattern_%s_bandwidth",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "url ptn %s",
		Ctx:      "web_log.url_pattern_bandwidth",
		Type:     module.Area,
		Priority: prioURLPatternStats + 2,
		Dims: Dims{
			{ID: "url_ptn_%s_bytes_received", Name: "received", Algo: module.Incremental, Mul: 8, Div: 1000},
			{ID: "url_ptn_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -8, Div: 1000},
		},
	}
	urlPatternReqProcTime = Chart{
		ID:       "url_pattern_%s_request_processing_time",
		Title:    "Request Processing Time",
		Units:    "milliseconds",
		Fam:      "url ptn %s",
		Ctx:      "web_log.url_pattern_request_processing_time",
		Priority: prioURLPatternStats + 3,
		Dims: Dims{
			{ID: "url_ptn_%s_req_proc_time_min", Name: "min", Div: 1000},
			{ID: "url_ptn_%s_req_proc_time_max", Name: "max", Div: 1000},
			{ID: "url_ptn_%s_req_proc_time_avg", Name: "avg", Div: 1000},
		},
	}
)

func newReqProcTimeHistChart(histogram []float64) (*Chart, error) {
	chart := reqProcTimeHist.Copy()
	for i, v := range histogram {
		dim := &Dim{
			ID:   fmt.Sprintf("req_proc_time_hist_bucket_%d", i+1),
			Name: fmt.Sprintf("%.3f", v),
			Algo: module.Incremental,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	if err := chart.AddDim(&Dim{
		ID:   "req_proc_time_hist_count",
		Name: "+Inf",
		Algo: module.Incremental,
	}); err != nil {
		return nil, err
	}
	return chart, nil
}

func newUpsRespTimeHistChart(histogram []float64) (*Chart, error) {
	chart := upsRespTimeHist.Copy()
	for i, v := range histogram {
		dim := &Dim{
			ID:   fmt.Sprintf("upstream_resp_time_hist_bucket_%d", i+1),
			Name: fmt.Sprintf("%.3f", v),
			Algo: module.Incremental,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	if err := chart.AddDim(&Dim{
		ID:   "upstream_resp_time_hist_count",
		Name: "+Inf",
		Algo: module.Incremental,
	}); err != nil {
		return nil, err
	}
	return chart, nil
}

func newURLPatternChart(patterns []userPattern) (*Chart, error) {
	chart := reqByURLPattern.Copy()
	for _, p := range patterns {
		dim := &Dim{
			ID:   "req_url_ptn_" + p.Name,
			Name: p.Name,
			Algo: module.Incremental,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	return chart, nil
}

func newURLPatternRespCodesChart(name string) *Chart {
	chart := urlPatternRespCodes.Copy()
	chart.ID = fmt.Sprintf(chart.ID, name)
	chart.Fam = fmt.Sprintf(chart.Fam, name)
	return chart
}

func newURLPatternReqMethodsChart(name string) *Chart {
	chart := urlPatternReqMethods.Copy()
	chart.ID = fmt.Sprintf(chart.ID, name)
	chart.Fam = fmt.Sprintf(chart.Fam, name)
	return chart
}

func newURLPatternBandwidthChart(name string) *Chart {
	chart := urlPatternBandwidth.Copy()
	chart.ID = fmt.Sprintf(chart.ID, name)
	chart.Fam = fmt.Sprintf(chart.Fam, name)
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, name)
	}
	return chart
}

func newURLPatternReqProcTimeChart(name string) *Chart {
	chart := urlPatternReqProcTime.Copy()
	chart.ID = fmt.Sprintf(chart.ID, name)
	chart.Fam = fmt.Sprintf(chart.Fam, name)
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, name)
	}
	return chart
}

func newCustomFieldCharts(fields []customField) (Charts, error) {
	charts := Charts{}
	for _, f := range fields {
		chart, err := newCustomFieldChart(f)
		if err != nil {
			return nil, err
		}
		if err := charts.Add(chart); err != nil {
			return nil, err
		}
	}
	return charts, nil
}

func newCustomFieldChart(f customField) (*Chart, error) {
	chart := reqByCustomFieldPattern.Copy()
	chart.ID = fmt.Sprintf(chart.ID, f.Name)
	chart.Title = fmt.Sprintf(chart.Title, f.Name)
	for _, p := range f.Patterns {
		dim := &Dim{
			ID:   fmt.Sprintf("custom_field_%s_%s", f.Name, p.Name),
			Name: p.Name,
			Algo: module.Incremental,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	return chart, nil
}

func newCustomTimeFieldCharts(fields []customTimeField) (Charts, error) {
	charts := Charts{}
	for i, f := range fields {
		chartTime, err := newCustomTimeFieldChart(f)
		if err != nil {
			return nil, err
		}
		chartTime.Priority += i
		if err := charts.Add(chartTime); err != nil {
			return nil, err
		}
		if len(f.Histogram) < 1 {
			continue
		}

		chartHist, err := newCustomTimeFieldHistChart(f)
		if err != nil {
			return nil, err
		}
		chartHist.Priority += i

		if err := charts.Add(chartHist); err != nil {
			return nil, err
		}
	}
	return charts, nil
}

func newCustomTimeFieldChart(f customTimeField) (*Chart, error) {
	chart := reqByCustomTimeField.Copy()
	chart.ID = fmt.Sprintf(chart.ID, f.Name)
	chart.Title = fmt.Sprintf(chart.Title, f.Name)
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, f.Name)
	}
	return chart, nil
}

func newCustomTimeFieldHistChart(f customTimeField) (*Chart, error) {
	chart := reqByCustomTimeFieldHist.Copy()
	chart.ID = fmt.Sprintf(chart.ID, f.Name)
	chart.Title = fmt.Sprintf(chart.Title, f.Name)
	for i, v := range f.Histogram {
		dim := &Dim{
			ID:   fmt.Sprintf("custom_time_field_%s_time_hist_bucket_%d", f.Name, i+1),
			Name: fmt.Sprintf("%.3f", v),
			Algo: module.Incremental,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	if err := chart.AddDim(&Dim{
		ID:   fmt.Sprintf("custom_time_field_%s_time_hist_count", f.Name),
		Name: "+Inf",
		Algo: module.Incremental,
	}); err != nil {
		return nil, err
	}
	return chart, nil
}

func (c *Collector) createCharts(line *logLine) error {
	if line.empty() {
		return errors.New("empty line")
	}
	c.charts = nil
	// Following charts are created during runtime:
	//   - reqBySSLProto, reqBySSLCipherSuite - it is likely line has no SSL stuff at this moment
	charts := &Charts{
		reqTotal.Copy(),
		reqExcluded.Copy(),
	}
	if line.hasVhost() {
		if err := addVhostCharts(charts); err != nil {
			return err
		}
	}
	if line.hasPort() {
		if err := addPortCharts(charts); err != nil {
			return err
		}
	}
	if line.hasReqScheme() {
		if err := addSchemeCharts(charts); err != nil {
			return err
		}
	}
	if line.hasReqClient() {
		if err := addClientCharts(charts); err != nil {
			return err
		}
	}
	if line.hasReqMethod() {
		if err := addMethodCharts(charts, c.URLPatterns); err != nil {
			return err
		}
	}
	if line.hasReqURL() {
		if err := addURLCharts(charts, c.URLPatterns); err != nil {
			return err
		}
	}
	if line.hasReqProto() {
		if err := addReqProtoCharts(charts); err != nil {
			return err
		}
	}
	if line.hasRespCode() {
		if err := addRespCodesCharts(charts, c.GroupRespCodes); err != nil {
			return err
		}
	}
	if line.hasReqSize() || line.hasRespSize() {
		if err := addBandwidthCharts(charts, c.URLPatterns); err != nil {
			return err
		}
	}
	if line.hasReqProcTime() {
		if err := addReqProcTimeCharts(charts, c.Histogram, c.URLPatterns); err != nil {
			return err
		}
	}
	if line.hasUpsRespTime() {
		if err := addUpstreamRespTimeCharts(charts, c.Histogram); err != nil {
			return err
		}
	}
	if line.hasCustomFields() {
		if len(c.CustomFields) > 0 {
			if err := addCustomFieldsCharts(charts, c.CustomFields); err != nil {
				return err
			}
		}
		if len(c.CustomTimeFields) > 0 {
			if err := addCustomTimeFieldsCharts(charts, c.CustomTimeFields); err != nil {
				return err
			}
		}
		if len(c.CustomNumericFields) > 0 {
			if err := addCustomNumericFieldsCharts(charts, c.CustomNumericFields); err != nil {
				return err
			}
		}
	}

	c.charts = charts

	return nil
}

func addVhostCharts(charts *Charts) error {
	return charts.Add(reqByVhost.Copy())
}

func addPortCharts(charts *Charts) error {
	return charts.Add(reqByPort.Copy())
}

func addSchemeCharts(charts *Charts) error {
	return charts.Add(reqByScheme.Copy())
}

func addClientCharts(charts *Charts) error {
	if err := charts.Add(reqByIPProto.Copy()); err != nil {
		return err
	}
	return charts.Add(uniqIPsCurPoll.Copy())
}

func addMethodCharts(charts *Charts, patterns []userPattern) error {
	if err := charts.Add(reqByMethod.Copy()); err != nil {
		return err
	}

	for _, p := range patterns {
		chart := newURLPatternReqMethodsChart(p.Name)
		if err := charts.Add(chart); err != nil {
			return err
		}
	}
	return nil
}

func addURLCharts(charts *Charts, patterns []userPattern) error {
	if len(patterns) == 0 {
		return nil
	}
	chart, err := newURLPatternChart(patterns)
	if err != nil {
		return err
	}
	if err := charts.Add(chart); err != nil {
		return err
	}

	for _, p := range patterns {
		chart := newURLPatternRespCodesChart(p.Name)
		if err := charts.Add(chart); err != nil {
			return err
		}
	}
	return nil
}

func addReqProtoCharts(charts *Charts) error {
	return charts.Add(reqByVersion.Copy())
}

func addRespCodesCharts(charts *Charts, group bool) error {
	if err := charts.Add(reqTypes.Copy()); err != nil {
		return err
	}
	if err := charts.Add(respCodeClass.Copy()); err != nil {
		return err
	}
	if !group {
		return charts.Add(respCodes.Copy())
	}
	for _, c := range []Chart{respCodes1xx, respCodes2xx, respCodes3xx, respCodes4xx, respCodes5xx} {
		if err := charts.Add(c.Copy()); err != nil {
			return err
		}
	}
	return nil
}

func addBandwidthCharts(charts *Charts, patterns []userPattern) error {
	if err := charts.Add(bandwidth.Copy()); err != nil {
		return err
	}

	for _, p := range patterns {
		chart := newURLPatternBandwidthChart(p.Name)
		if err := charts.Add(chart); err != nil {
			return err
		}
	}
	return nil
}

func addReqProcTimeCharts(charts *Charts, histogram []float64, patterns []userPattern) error {
	if err := charts.Add(reqProcTime.Copy()); err != nil {
		return err
	}
	for _, p := range patterns {
		chart := newURLPatternReqProcTimeChart(p.Name)
		if err := charts.Add(chart); err != nil {
			return err
		}
	}
	if len(histogram) == 0 {
		return nil
	}
	chart, err := newReqProcTimeHistChart(histogram)
	if err != nil {
		return err
	}
	return charts.Add(chart)
}

func addUpstreamRespTimeCharts(charts *Charts, histogram []float64) error {
	if err := charts.Add(upsRespTime.Copy()); err != nil {
		return err
	}
	if len(histogram) == 0 {
		return nil
	}
	chart, err := newUpsRespTimeHistChart(histogram)
	if err != nil {
		return err
	}
	return charts.Add(chart)
}

func addCustomFieldsCharts(charts *Charts, fields []customField) error {
	cs, err := newCustomFieldCharts(fields)
	if err != nil {
		return err
	}
	return charts.Add(cs...)
}

func addCustomTimeFieldsCharts(charts *Charts, fields []customTimeField) error {
	cs, err := newCustomTimeFieldCharts(fields)
	if err != nil {
		return err
	}
	return charts.Add(cs...)
}

func addCustomNumericFieldsCharts(charts *module.Charts, fields []customNumericField) error {
	for _, f := range fields {
		chart := customNumericFieldSummaryChartTmpl.Copy()
		chart.ID = fmt.Sprintf(chart.ID, f.Name)
		chart.Units = f.Units
		chart.Ctx = fmt.Sprintf(chart.Ctx, f.Name)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, f.Name)
			dim.Div = f.Divisor
		}

		if err := charts.Add(chart); err != nil {
			return err
		}
	}

	return nil
}
