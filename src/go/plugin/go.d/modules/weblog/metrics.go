// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func newWebLogSummary() metrix.Summary {
	return &weblogSummary{metrix.NewSummary()}
}

type weblogSummary struct {
	metrix.Summary
}

// WriteTo redefines metrics.Summary.WriteTo
// TODO: temporary workaround?
func (s weblogSummary) WriteTo(rv map[string]int64, key string, mul, div int) {
	s.Summary.WriteTo(rv, key, mul, div)
	if _, ok := rv[key+"_min"]; !ok {
		rv[key+"_min"] = 0
		rv[key+"_max"] = 0
		rv[key+"_avg"] = 0
	}
}

type (
	metricsData struct {
		Requests     metrix.Counter `stm:"requests"`
		ReqUnmatched metrix.Counter `stm:"req_unmatched"`

		RespCode metrix.CounterVec `stm:"resp_code"`
		Resp1xx  metrix.Counter    `stm:"resp_1xx"`
		Resp2xx  metrix.Counter    `stm:"resp_2xx"`
		Resp3xx  metrix.Counter    `stm:"resp_3xx"`
		Resp4xx  metrix.Counter    `stm:"resp_4xx"`
		Resp5xx  metrix.Counter    `stm:"resp_5xx"`

		ReqSuccess  metrix.Counter `stm:"req_type_success"`
		ReqRedirect metrix.Counter `stm:"req_type_redirect"`
		ReqBad      metrix.Counter `stm:"req_type_bad"`
		ReqError    metrix.Counter `stm:"req_type_error"`

		UniqueIPv4      metrix.UniqueCounter `stm:"uniq_ipv4"`
		UniqueIPv6      metrix.UniqueCounter `stm:"uniq_ipv6"`
		BytesSent       metrix.Counter       `stm:"bytes_sent"`
		BytesReceived   metrix.Counter       `stm:"bytes_received"`
		ReqProcTime     metrix.Summary       `stm:"req_proc_time"`
		ReqProcTimeHist metrix.Histogram     `stm:"req_proc_time_hist"`
		UpsRespTime     metrix.Summary       `stm:"upstream_resp_time"`
		UpsRespTimeHist metrix.Histogram     `stm:"upstream_resp_time_hist"`

		ReqVhost          metrix.CounterVec `stm:"req_vhost"`
		ReqPort           metrix.CounterVec `stm:"req_port"`
		ReqMethod         metrix.CounterVec `stm:"req_method"`
		ReqURLPattern     metrix.CounterVec `stm:"req_url_ptn"`
		ReqVersion        metrix.CounterVec `stm:"req_version"`
		ReqSSLProto       metrix.CounterVec `stm:"req_ssl_proto"`
		ReqSSLCipherSuite metrix.CounterVec `stm:"req_ssl_cipher_suite"`
		ReqHTTPScheme     metrix.Counter    `stm:"req_http_scheme"`
		ReqHTTPSScheme    metrix.Counter    `stm:"req_https_scheme"`
		ReqIPv4           metrix.Counter    `stm:"req_ipv4"`
		ReqIPv6           metrix.Counter    `stm:"req_ipv6"`

		ReqCustomField  map[string]metrix.CounterVec `stm:"custom_field"`
		URLPatternStats map[string]*patternMetrics   `stm:"url_ptn"`

		ReqCustomTimeField    map[string]*customTimeFieldMetrics    `stm:"custom_time_field"`
		ReqCustomNumericField map[string]*customNumericFieldMetrics `stm:"custom_numeric_field"`
	}
	customTimeFieldMetrics struct {
		Time     metrix.Summary   `stm:"time"`
		TimeHist metrix.Histogram `stm:"time_hist"`
	}
	customNumericFieldMetrics struct {
		Summary metrix.Summary `stm:"summary"`

		multiplier int
		divisor    int
	}
	patternMetrics struct {
		RespCode      metrix.CounterVec `stm:"resp_code"`
		ReqMethod     metrix.CounterVec `stm:"req_method"`
		BytesSent     metrix.Counter    `stm:"bytes_sent"`
		BytesReceived metrix.Counter    `stm:"bytes_received"`
		ReqProcTime   metrix.Summary    `stm:"req_proc_time"`
	}
)

func newMetricsData(config Config) *metricsData {
	return &metricsData{
		ReqVhost:              metrix.NewCounterVec(),
		ReqPort:               metrix.NewCounterVec(),
		ReqMethod:             metrix.NewCounterVec(),
		ReqVersion:            metrix.NewCounterVec(),
		RespCode:              metrix.NewCounterVec(),
		ReqSSLProto:           metrix.NewCounterVec(),
		ReqSSLCipherSuite:     metrix.NewCounterVec(),
		ReqProcTime:           newWebLogSummary(),
		ReqProcTimeHist:       metrix.NewHistogram(convHistOptionsToMicroseconds(config.Histogram)),
		UpsRespTime:           newWebLogSummary(),
		UpsRespTimeHist:       metrix.NewHistogram(convHistOptionsToMicroseconds(config.Histogram)),
		UniqueIPv4:            metrix.NewUniqueCounter(true),
		UniqueIPv6:            metrix.NewUniqueCounter(true),
		ReqURLPattern:         newCounterVecFromPatterns(config.URLPatterns),
		ReqCustomField:        newReqCustomField(config.CustomFields),
		URLPatternStats:       newURLPatternStats(config.URLPatterns),
		ReqCustomTimeField:    newReqCustomTimeField(config.CustomTimeFields),
		ReqCustomNumericField: newReqCustomNumericField(config.CustomNumericFields),
	}
}

func (m *metricsData) reset() {
	m.UniqueIPv4.Reset()
	m.UniqueIPv6.Reset()
	m.ReqProcTime.Reset()
	m.UpsRespTime.Reset()
	for _, v := range m.URLPatternStats {
		v.ReqProcTime.Reset()
	}
	for _, v := range m.ReqCustomTimeField {
		v.Time.Reset()
	}
	for _, v := range m.ReqCustomNumericField {
		v.Summary.Reset()
	}
}

func newCounterVecFromPatterns(patterns []userPattern) metrix.CounterVec {
	c := metrix.NewCounterVec()
	for _, p := range patterns {
		_, _ = c.GetP(p.Name)
	}
	return c
}

func newURLPatternStats(patterns []userPattern) map[string]*patternMetrics {
	stats := make(map[string]*patternMetrics)
	for _, p := range patterns {
		stats[p.Name] = &patternMetrics{
			RespCode:    metrix.NewCounterVec(),
			ReqMethod:   metrix.NewCounterVec(),
			ReqProcTime: newWebLogSummary(),
		}
	}
	return stats
}

func newReqCustomField(fields []customField) map[string]metrix.CounterVec {
	cf := make(map[string]metrix.CounterVec)
	for _, f := range fields {
		cf[f.Name] = newCounterVecFromPatterns(f.Patterns)
	}
	return cf
}

func newReqCustomTimeField(fields []customTimeField) map[string]*customTimeFieldMetrics {
	cf := make(map[string]*customTimeFieldMetrics)
	for _, f := range fields {
		cf[f.Name] = &customTimeFieldMetrics{
			Time:     newWebLogSummary(),
			TimeHist: metrix.NewHistogram(convHistOptionsToMicroseconds(f.Histogram)),
		}
	}
	return cf
}

func newReqCustomNumericField(fields []customNumericField) map[string]*customNumericFieldMetrics {
	rv := make(map[string]*customNumericFieldMetrics)
	for _, f := range fields {
		rv[f.Name] = &customNumericFieldMetrics{
			Summary:    newWebLogSummary(),
			multiplier: f.Multiplier,
			divisor:    f.Divisor,
		}
	}
	return rv
}

// convert histogram options to microseconds (second => us)
func convHistOptionsToMicroseconds(histogram []float64) []float64 {
	var buckets []float64
	for _, value := range histogram {
		buckets = append(buckets, value*1e6)
	}
	return buckets
}
