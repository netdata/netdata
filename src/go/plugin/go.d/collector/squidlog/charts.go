// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"errors"

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

	prioHTTPRespCodesClass
	prioHTTPRespCodes

	prioUniqClients

	prioBandwidth

	prioRespTime

	prioCacheCode
	prioCacheTransportTag
	prioCacheHandlingTag
	prioCacheObjectTag
	prioCacheLoadSourceTag
	prioCacheErrorTag

	prioReqMethod

	prioHierCode
	prioServers

	prioMimeType
)

var (
	// Requests
	reqTotalChart = Chart{
		ID:       "requests",
		Title:    "Total Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "squidlog.requests",
		Priority: prioReqTotal,
		Dims: Dims{
			{ID: "requests", Algo: module.Incremental},
		},
	}
	reqExcludedChart = Chart{
		ID:       "excluded_requests",
		Title:    "Excluded Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "squidlog.excluded_requests",
		Priority: prioReqExcluded,
		Dims: Dims{
			{ID: "unmatched", Algo: module.Incremental},
		},
	}
	reqTypesChart = Chart{
		ID:       "requests_by_type",
		Title:    "Requests By Type",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "squidlog.type_requests",
		Type:     module.Stacked,
		Priority: prioReqType,
		Dims: Dims{
			{ID: "req_type_success", Name: "success", Algo: module.Incremental},
			{ID: "req_type_bad", Name: "bad", Algo: module.Incremental},
			{ID: "req_type_redirect", Name: "redirect", Algo: module.Incremental},
			{ID: "req_type_error", Name: "error", Algo: module.Incremental},
		},
	}

	// HTTP Code
	httpRespCodeClassChart = Chart{
		ID:       "responses_by_http_status_code_class",
		Title:    "Responses By HTTP Status Code Class",
		Units:    "responses/s",
		Fam:      "http code",
		Ctx:      "squidlog.http_status_code_class_responses",
		Type:     module.Stacked,
		Priority: prioHTTPRespCodesClass,
		Dims: Dims{
			{ID: "http_resp_2xx", Name: "2xx", Algo: module.Incremental},
			{ID: "http_resp_5xx", Name: "5xx", Algo: module.Incremental},
			{ID: "http_resp_3xx", Name: "3xx", Algo: module.Incremental},
			{ID: "http_resp_4xx", Name: "4xx", Algo: module.Incremental},
			{ID: "http_resp_1xx", Name: "1xx", Algo: module.Incremental},
			{ID: "http_resp_0xx", Name: "0xx", Algo: module.Incremental},
			{ID: "http_resp_6xx", Name: "6xx", Algo: module.Incremental},
		},
	}
	httpRespCodesChart = Chart{
		ID:       "responses_by_http_status_code",
		Title:    "Responses By HTTP Status Code",
		Units:    "responses/s",
		Fam:      "http code",
		Ctx:      "squidlog.http_status_code_responses",
		Type:     module.Stacked,
		Priority: prioHTTPRespCodes,
	}

	// Bandwidth
	bandwidthChart = Chart{
		ID:       "bandwidth",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "bandwidth",
		Ctx:      "squidlog.bandwidth",
		Priority: prioBandwidth,
		Dims: Dims{
			{ID: "bytes_sent", Name: "sent", Algo: module.Incremental, Div: 1000},
		},
	}

	// Response Time
	respTimeChart = Chart{
		ID:       "response_time",
		Title:    "Response Time",
		Units:    "milliseconds",
		Fam:      "timings",
		Ctx:      "squidlog.response_time",
		Priority: prioRespTime,
		Dims: Dims{
			{ID: "resp_time_min", Name: "min", Div: 1000},
			{ID: "resp_time_max", Name: "max", Div: 1000},
			{ID: "resp_time_avg", Name: "avg", Div: 1000},
		},
	}

	// Clients
	uniqClientsChart = Chart{
		ID:       "uniq_clients",
		Title:    "Unique Clients",
		Units:    "clients/s",
		Fam:      "clients",
		Ctx:      "squidlog.uniq_clients",
		Priority: prioUniqClients,
		Dims: Dims{
			{ID: "uniq_clients", Name: "clients"},
		},
	}

	// Cache Code Result
	cacheCodeChart = Chart{
		ID:       "requests_by_cache_result_code",
		Title:    "Requests By Cache Result Code",
		Units:    "requests/s",
		Fam:      "cache result",
		Ctx:      "squidlog.cache_result_code_requests",
		Priority: prioCacheCode,
		Type:     module.Stacked,
	}
	cacheCodeTransportTagChart = Chart{
		ID:       "requests_by_cache_result_code_transport_tag",
		Title:    "Requests By Cache Result Delivery Transport Tag",
		Units:    "requests/s",
		Fam:      "cache result",
		Ctx:      "squidlog.cache_result_code_transport_tag_requests",
		Type:     module.Stacked,
		Priority: prioCacheTransportTag,
	}
	cacheCodeHandlingTagChart = Chart{
		ID:       "requests_by_cache_result_code_handling_tag",
		Title:    "Requests By Cache Result Handling Tag",
		Units:    "requests/s",
		Fam:      "cache result",
		Ctx:      "squidlog.cache_result_code_handling_tag_requests",
		Type:     module.Stacked,
		Priority: prioCacheHandlingTag,
	}
	cacheCodeObjectTagChart = Chart{
		ID:       "requests_by_cache_code_object_tag",
		Title:    "Requests By Cache Result Produced Object Tag",
		Units:    "requests/s",
		Fam:      "cache result",
		Ctx:      "squidlog.cache_code_object_tag_requests",
		Type:     module.Stacked,
		Priority: prioCacheObjectTag,
	}
	cacheCodeLoadSourceTagChart = Chart{
		ID:       "requests_by_cache_code_load_source_tag",
		Title:    "Requests By Cache Result Load Source Tag",
		Units:    "requests/s",
		Fam:      "cache result",
		Ctx:      "squidlog.cache_code_load_source_tag_requests",
		Type:     module.Stacked,
		Priority: prioCacheLoadSourceTag,
	}
	cacheCodeErrorTagChart = Chart{
		ID:       "requests_by_cache_code_error_tag",
		Title:    "Requests By Cache Result Errors Tag",
		Units:    "requests/s",
		Fam:      "cache result",
		Ctx:      "squidlog.cache_code_error_tag_requests",
		Type:     module.Stacked,
		Priority: prioCacheErrorTag,
	}

	// HTTP Method
	reqMethodChart = Chart{
		ID:       "requests_by_http_method",
		Title:    "Requests By HTTP Method",
		Units:    "requests/s",
		Fam:      "http method",
		Ctx:      "squidlog.http_method_requests",
		Type:     module.Stacked,
		Priority: prioReqMethod,
	}

	// MIME Type
	mimeTypeChart = Chart{
		ID:       "requests_by_mime_type",
		Title:    "Requests By MIME Type",
		Units:    "requests/s",
		Fam:      "mime type",
		Ctx:      "squidlog.mime_type_requests",
		Type:     module.Stacked,
		Priority: prioMimeType,
	}

	// Hierarchy
	hierCodeChart = Chart{
		ID:       "requests_by_hier_code",
		Title:    "Requests By Hierarchy Code",
		Units:    "requests/s",
		Fam:      "hierarchy",
		Ctx:      "squidlog.hier_code_requests",
		Type:     module.Stacked,
		Priority: prioHierCode,
	}
	serverAddrChart = Chart{
		ID:       "forwarded_requests_by_server_address",
		Title:    "Forwarded Requests By Server Address",
		Units:    "requests/s",
		Fam:      "hierarchy",
		Ctx:      "squidlog.server_address_forwarded_requests",
		Type:     module.Stacked,
		Priority: prioServers,
	}
)

func (c *Collector) createCharts(line *logLine) error {
	if line.empty() {
		return errors.New("empty line")
	}
	charts := &Charts{
		reqTotalChart.Copy(),
		reqExcludedChart.Copy(),
	}
	if line.hasRespTime() {
		if err := addRespTimeCharts(charts); err != nil {
			return err
		}
	}
	if line.hasClientAddress() {
		if err := addClientAddressCharts(charts); err != nil {
			return err
		}
	}
	if line.hasCacheCode() {
		if err := addCacheCodeCharts(charts); err != nil {
			return err
		}
	}
	if line.hasHTTPCode() {
		if err := addHTTPRespCodeCharts(charts); err != nil {
			return err
		}
	}
	if line.hasRespSize() {
		if err := addRespSizeCharts(charts); err != nil {
			return err
		}
	}
	if line.hasReqMethod() {
		if err := addMethodCharts(charts); err != nil {
			return err
		}
	}
	if line.hasHierCode() {
		if err := addHierCodeCharts(charts); err != nil {
			return err
		}
	}
	if line.hasServerAddress() {
		if err := addServerAddressCharts(charts); err != nil {
			return err
		}
	}
	if line.hasMimeType() {
		if err := addMimeTypeCharts(charts); err != nil {
			return err
		}
	}
	c.charts = charts
	return nil
}

func addRespTimeCharts(charts *Charts) error {
	return charts.Add(respTimeChart.Copy())
}

func addClientAddressCharts(charts *Charts) error {
	return charts.Add(uniqClientsChart.Copy())
}

func addCacheCodeCharts(charts *Charts) error {
	cs := []Chart{
		cacheCodeChart,
		cacheCodeTransportTagChart,
		cacheCodeHandlingTagChart,
		cacheCodeObjectTagChart,
		cacheCodeLoadSourceTagChart,
		cacheCodeErrorTagChart,
	}
	for _, chart := range cs {
		if err := charts.Add(chart.Copy()); err != nil {
			return err
		}
	}
	return nil
}
func addHTTPRespCodeCharts(charts *Charts) error {
	cs := []Chart{
		reqTypesChart,
		httpRespCodeClassChart,
		httpRespCodesChart,
	}
	for _, chart := range cs {
		if err := charts.Add(chart.Copy()); err != nil {
			return err
		}
	}
	return nil
}

func addRespSizeCharts(charts *Charts) error {
	return charts.Add(bandwidthChart.Copy())
}

func addMethodCharts(charts *Charts) error {
	return charts.Add(reqMethodChart.Copy())
}

func addHierCodeCharts(charts *Charts) error {
	return charts.Add(hierCodeChart.Copy())
}
func addServerAddressCharts(charts *Charts) error {
	return charts.Add(serverAddrChart.Copy())
}

func addMimeTypeCharts(charts *Charts) error {
	return charts.Add(mimeTypeChart.Copy())
}
