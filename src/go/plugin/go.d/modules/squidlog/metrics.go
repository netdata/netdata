// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

func newSummary() metrix.Summary {
	return &summary{metrix.NewSummary()}
}

type summary struct {
	metrix.Summary
}

func (s summary) WriteTo(rv map[string]int64, key string, mul, div int) {
	s.Summary.WriteTo(rv, key, mul, div)
	if _, ok := rv[key+"_min"]; !ok {
		rv[key+"_min"] = 0
		rv[key+"_max"] = 0
		rv[key+"_avg"] = 0
	}
}

const (
	pxHTTPCode     = "http_resp_code_"
	pxReqMethod    = "req_method_"
	pxCacheCode    = "cache_result_code_"
	pxTransportTag = "cache_transport_tag_"
	pxHandlingTag  = "cache_handling_tag_"
	pxObjectTag    = "cache_object_tag_"
	pxSourceTag    = "cache_load_source_tag_"
	pxErrorTag     = "cache_error_tag_"
	pxHierCode     = "hier_code_"
	pxMimeType     = "mime_type_"
	pxSrvAddr      = "server_address_"
)

type metricsData struct {
	Requests  metrix.Counter `stm:"requests"`
	Unmatched metrix.Counter `stm:"unmatched"`

	HTTPRespCode metrix.CounterVec `stm:"http_resp_code"`
	HTTPResp0xx  metrix.Counter    `stm:"http_resp_0xx"`
	HTTPResp1xx  metrix.Counter    `stm:"http_resp_1xx"`
	HTTPResp2xx  metrix.Counter    `stm:"http_resp_2xx"`
	HTTPResp3xx  metrix.Counter    `stm:"http_resp_3xx"`
	HTTPResp4xx  metrix.Counter    `stm:"http_resp_4xx"`
	HTTPResp5xx  metrix.Counter    `stm:"http_resp_5xx"`
	HTTPResp6xx  metrix.Counter    `stm:"http_resp_6xx"`

	ReqSuccess  metrix.Counter `stm:"req_type_success"`
	ReqRedirect metrix.Counter `stm:"req_type_redirect"`
	ReqBad      metrix.Counter `stm:"req_type_bad"`
	ReqError    metrix.Counter `stm:"req_type_error"`

	BytesSent     metrix.Counter       `stm:"bytes_sent"`
	RespTime      metrix.Summary       `stm:"resp_time,1000,1"`
	UniqueClients metrix.UniqueCounter `stm:"uniq_clients"`

	ReqMethod              metrix.CounterVec `stm:"req_method"`
	CacheCode              metrix.CounterVec `stm:"cache_result_code"`
	CacheCodeTransportTag  metrix.CounterVec `stm:"cache_transport_tag"`
	CacheCodeHandlingTag   metrix.CounterVec `stm:"cache_handling_tag"`
	CacheCodeObjectTag     metrix.CounterVec `stm:"cache_object_tag"`
	CacheCodeLoadSourceTag metrix.CounterVec `stm:"cache_load_source_tag"`
	CacheCodeErrorTag      metrix.CounterVec `stm:"cache_error_tag"`
	HierCode               metrix.CounterVec `stm:"hier_code"`
	MimeType               metrix.CounterVec `stm:"mime_type"`
	Server                 metrix.CounterVec `stm:"server_address"`
}

func (m *metricsData) reset() {
	m.RespTime.Reset()
	m.UniqueClients.Reset()
}

func newMetricsData() *metricsData {
	return &metricsData{
		RespTime:               newSummary(),
		UniqueClients:          metrix.NewUniqueCounter(true),
		HTTPRespCode:           metrix.NewCounterVec(),
		ReqMethod:              metrix.NewCounterVec(),
		CacheCode:              metrix.NewCounterVec(),
		CacheCodeTransportTag:  metrix.NewCounterVec(),
		CacheCodeHandlingTag:   metrix.NewCounterVec(),
		CacheCodeObjectTag:     metrix.NewCounterVec(),
		CacheCodeLoadSourceTag: metrix.NewCounterVec(),
		CacheCodeErrorTag:      metrix.NewCounterVec(),
		HierCode:               metrix.NewCounterVec(),
		Server:                 metrix.NewCounterVec(),
		MimeType:               metrix.NewCounterVec(),
	}
}
