// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrics"
)

func newWebLogSummary() metrics.Summary {
	return &weblogSummary{metrics.NewSummary()}
}

type weblogSummary struct {
	metrics.Summary
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
		Requests     metrics.Counter `stm:"requests"`
		ReqUnmatched metrics.Counter `stm:"req_unmatched"`

		RespCode metrics.CounterVec `stm:"resp_code"`
		Resp1xx  metrics.Counter    `stm:"resp_1xx"`
		Resp2xx  metrics.Counter    `stm:"resp_2xx"`
		Resp3xx  metrics.Counter    `stm:"resp_3xx"`
		Resp4xx  metrics.Counter    `stm:"resp_4xx"`
		Resp5xx  metrics.Counter    `stm:"resp_5xx"`

		ReqSuccess  metrics.Counter `stm:"req_type_success"`
		ReqRedirect metrics.Counter `stm:"req_type_redirect"`
		ReqBad      metrics.Counter `stm:"req_type_bad"`
		ReqError    metrics.Counter `stm:"req_type_error"`

		UniqueIPv4      metrics.UniqueCounter `stm:"uniq_ipv4"`
		UniqueIPv6      metrics.UniqueCounter `stm:"uniq_ipv6"`
		BytesSent       metrics.Counter       `stm:"bytes_sent"`
		BytesReceived   metrics.Counter       `stm:"bytes_received"`
		ReqProcTime     metrics.Summary       `stm:"req_proc_time"`
		ReqProcTimeHist metrics.Histogram     `stm:"req_proc_time_hist"`
		UpsRespTime     metrics.Summary       `stm:"upstream_resp_time"`
		UpsRespTimeHist metrics.Histogram     `stm:"upstream_resp_time_hist"`

		ReqVhost          metrics.CounterVec `stm:"req_vhost"`
		ReqPort           metrics.CounterVec `stm:"req_port"`
		ReqMethod         metrics.CounterVec `stm:"req_method"`
		ReqURLPattern     metrics.CounterVec `stm:"req_url_ptn"`
		ReqVersion        metrics.CounterVec `stm:"req_version"`
		ReqSSLProto       metrics.CounterVec `stm:"req_ssl_proto"`
		ReqSSLCipherSuite metrics.CounterVec `stm:"req_ssl_cipher_suite"`
		ReqHTTPScheme     metrics.Counter    `stm:"req_http_scheme"`
		ReqHTTPSScheme    metrics.Counter    `stm:"req_https_scheme"`
		ReqIPv4           metrics.Counter    `stm:"req_ipv4"`
		ReqIPv6           metrics.Counter    `stm:"req_ipv6"`

		ReqCustomField  map[string]metrics.CounterVec `stm:"custom_field"`
		URLPatternStats map[string]*patternMetrics    `stm:"url_ptn"`

		ReqCustomTimeField    map[string]*customTimeFieldMetrics    `stm:"custom_time_field"`
		ReqCustomNumericField map[string]*customNumericFieldMetrics `stm:"custom_numeric_field"`
	}
	customTimeFieldMetrics struct {
		Time     metrics.Summary   `stm:"time"`
		TimeHist metrics.Histogram `stm:"time_hist"`
	}
	customNumericFieldMetrics struct {
		Summary metrics.Summary `stm:"summary"`

		multiplier int
		divisor    int
	}
	patternMetrics struct {
		RespCode      metrics.CounterVec `stm:"resp_code"`
		ReqMethod     metrics.CounterVec `stm:"req_method"`
		BytesSent     metrics.Counter    `stm:"bytes_sent"`
		BytesReceived metrics.Counter    `stm:"bytes_received"`
		ReqProcTime   metrics.Summary    `stm:"req_proc_time"`
	}
)

func newMetricsData(config Config) *metricsData {
	return &metricsData{
		ReqVhost:              metrics.NewCounterVec(),
		ReqPort:               metrics.NewCounterVec(),
		ReqMethod:             metrics.NewCounterVec(),
		ReqVersion:            metrics.NewCounterVec(),
		RespCode:              metrics.NewCounterVec(),
		ReqSSLProto:           metrics.NewCounterVec(),
		ReqSSLCipherSuite:     metrics.NewCounterVec(),
		ReqProcTime:           newWebLogSummary(),
		ReqProcTimeHist:       metrics.NewHistogram(convHistOptionsToMicroseconds(config.Histogram)),
		UpsRespTime:           newWebLogSummary(),
		UpsRespTimeHist:       metrics.NewHistogram(convHistOptionsToMicroseconds(config.Histogram)),
		UniqueIPv4:            metrics.NewUniqueCounter(true),
		UniqueIPv6:            metrics.NewUniqueCounter(true),
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

func newCounterVecFromPatterns(patterns []userPattern) metrics.CounterVec {
	c := metrics.NewCounterVec()
	for _, p := range patterns {
		_, _ = c.GetP(p.Name)
	}
	return c
}

func newURLPatternStats(patterns []userPattern) map[string]*patternMetrics {
	stats := make(map[string]*patternMetrics)
	for _, p := range patterns {
		stats[p.Name] = &patternMetrics{
			RespCode:    metrics.NewCounterVec(),
			ReqMethod:   metrics.NewCounterVec(),
			ReqProcTime: newWebLogSummary(),
		}
	}
	return stats
}

func newReqCustomField(fields []customField) map[string]metrics.CounterVec {
	cf := make(map[string]metrics.CounterVec)
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
			TimeHist: metrics.NewHistogram(convHistOptionsToMicroseconds(f.Histogram)),
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
