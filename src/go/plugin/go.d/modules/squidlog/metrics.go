// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrics"

func newSummary() metrics.Summary {
	return &summary{metrics.NewSummary()}
}

type summary struct {
	metrics.Summary
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
	Requests  metrics.Counter `stm:"requests"`
	Unmatched metrics.Counter `stm:"unmatched"`

	HTTPRespCode metrics.CounterVec `stm:"http_resp_code"`
	HTTPResp0xx  metrics.Counter    `stm:"http_resp_0xx"`
	HTTPResp1xx  metrics.Counter    `stm:"http_resp_1xx"`
	HTTPResp2xx  metrics.Counter    `stm:"http_resp_2xx"`
	HTTPResp3xx  metrics.Counter    `stm:"http_resp_3xx"`
	HTTPResp4xx  metrics.Counter    `stm:"http_resp_4xx"`
	HTTPResp5xx  metrics.Counter    `stm:"http_resp_5xx"`
	HTTPResp6xx  metrics.Counter    `stm:"http_resp_6xx"`

	ReqSuccess  metrics.Counter `stm:"req_type_success"`
	ReqRedirect metrics.Counter `stm:"req_type_redirect"`
	ReqBad      metrics.Counter `stm:"req_type_bad"`
	ReqError    metrics.Counter `stm:"req_type_error"`

	BytesSent     metrics.Counter       `stm:"bytes_sent"`
	RespTime      metrics.Summary       `stm:"resp_time,1000,1"`
	UniqueClients metrics.UniqueCounter `stm:"uniq_clients"`

	ReqMethod              metrics.CounterVec `stm:"req_method"`
	CacheCode              metrics.CounterVec `stm:"cache_result_code"`
	CacheCodeTransportTag  metrics.CounterVec `stm:"cache_transport_tag"`
	CacheCodeHandlingTag   metrics.CounterVec `stm:"cache_handling_tag"`
	CacheCodeObjectTag     metrics.CounterVec `stm:"cache_object_tag"`
	CacheCodeLoadSourceTag metrics.CounterVec `stm:"cache_load_source_tag"`
	CacheCodeErrorTag      metrics.CounterVec `stm:"cache_error_tag"`
	HierCode               metrics.CounterVec `stm:"hier_code"`
	MimeType               metrics.CounterVec `stm:"mime_type"`
	Server                 metrics.CounterVec `stm:"server_address"`
}

func (m *metricsData) reset() {
	m.RespTime.Reset()
	m.UniqueClients.Reset()
}

func newMetricsData() *metricsData {
	return &metricsData{
		RespTime:               newSummary(),
		UniqueClients:          metrics.NewUniqueCounter(true),
		HTTPRespCode:           metrics.NewCounterVec(),
		ReqMethod:              metrics.NewCounterVec(),
		CacheCode:              metrics.NewCounterVec(),
		CacheCodeTransportTag:  metrics.NewCounterVec(),
		CacheCodeHandlingTag:   metrics.NewCounterVec(),
		CacheCodeObjectTag:     metrics.NewCounterVec(),
		CacheCodeLoadSourceTag: metrics.NewCounterVec(),
		CacheCodeErrorTag:      metrics.NewCounterVec(),
		HierCode:               metrics.NewCounterVec(),
		Server:                 metrics.NewCounterVec(),
		MimeType:               metrics.NewCounterVec(),
	}
}
