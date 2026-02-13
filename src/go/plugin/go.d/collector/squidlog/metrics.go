// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"

func newSummary() oldmetrix.Summary {
	return &summary{oldmetrix.NewSummary()}
}

type summary struct {
	oldmetrix.Summary
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
	Requests  oldmetrix.Counter `stm:"requests"`
	Unmatched oldmetrix.Counter `stm:"unmatched"`

	HTTPRespCode oldmetrix.CounterVec `stm:"http_resp_code"`
	HTTPResp0xx  oldmetrix.Counter    `stm:"http_resp_0xx"`
	HTTPResp1xx  oldmetrix.Counter    `stm:"http_resp_1xx"`
	HTTPResp2xx  oldmetrix.Counter    `stm:"http_resp_2xx"`
	HTTPResp3xx  oldmetrix.Counter    `stm:"http_resp_3xx"`
	HTTPResp4xx  oldmetrix.Counter    `stm:"http_resp_4xx"`
	HTTPResp5xx  oldmetrix.Counter    `stm:"http_resp_5xx"`
	HTTPResp6xx  oldmetrix.Counter    `stm:"http_resp_6xx"`

	ReqSuccess  oldmetrix.Counter `stm:"req_type_success"`
	ReqRedirect oldmetrix.Counter `stm:"req_type_redirect"`
	ReqBad      oldmetrix.Counter `stm:"req_type_bad"`
	ReqError    oldmetrix.Counter `stm:"req_type_error"`

	BytesSent     oldmetrix.Counter       `stm:"bytes_sent"`
	RespTime      oldmetrix.Summary       `stm:"resp_time,1000,1"`
	UniqueClients oldmetrix.UniqueCounter `stm:"uniq_clients"`

	ReqMethod              oldmetrix.CounterVec `stm:"req_method"`
	CacheCode              oldmetrix.CounterVec `stm:"cache_result_code"`
	CacheCodeTransportTag  oldmetrix.CounterVec `stm:"cache_transport_tag"`
	CacheCodeHandlingTag   oldmetrix.CounterVec `stm:"cache_handling_tag"`
	CacheCodeObjectTag     oldmetrix.CounterVec `stm:"cache_object_tag"`
	CacheCodeLoadSourceTag oldmetrix.CounterVec `stm:"cache_load_source_tag"`
	CacheCodeErrorTag      oldmetrix.CounterVec `stm:"cache_error_tag"`
	HierCode               oldmetrix.CounterVec `stm:"hier_code"`
	MimeType               oldmetrix.CounterVec `stm:"mime_type"`
	Server                 oldmetrix.CounterVec `stm:"server_address"`
}

func (m *metricsData) reset() {
	m.RespTime.Reset()
	m.UniqueClients.Reset()
}

func newMetricsData() *metricsData {
	return &metricsData{
		RespTime:               newSummary(),
		UniqueClients:          oldmetrix.NewUniqueCounter(true),
		HTTPRespCode:           oldmetrix.NewCounterVec(),
		ReqMethod:              oldmetrix.NewCounterVec(),
		CacheCode:              oldmetrix.NewCounterVec(),
		CacheCodeTransportTag:  oldmetrix.NewCounterVec(),
		CacheCodeHandlingTag:   oldmetrix.NewCounterVec(),
		CacheCodeObjectTag:     oldmetrix.NewCounterVec(),
		CacheCodeLoadSourceTag: oldmetrix.NewCounterVec(),
		CacheCodeErrorTag:      oldmetrix.NewCounterVec(),
		HierCode:               oldmetrix.NewCounterVec(),
		Server:                 oldmetrix.NewCounterVec(),
		MimeType:               oldmetrix.NewCounterVec(),
	}
}
