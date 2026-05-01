// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"errors"
	"io/fs"
	"net"
	"strings"
	"syscall"
	"time"
)

const (
	collectorStatusOK              = "ok"
	collectorStatusPermissionError = "permission_error"
	collectorStatusTimeout         = "timeout"
	collectorStatusQueryError      = "query_error"
	collectorStatusParseError      = "parse_error"
)

var errFeatureUnsupported = errors.New("feature not supported")

type scrapeMetrics struct {
	status string

	scrapeDurationMs int64

	queryErrors     int64
	parseErrors     int64
	deepQueryErrors int64

	deepQueriesAttempted int64
	deepQueriesFailed    int64
	deepQueriesSkipped   int64
}

func newScrapeMetrics() scrapeMetrics {
	return scrapeMetrics{status: collectorStatusOK}
}

func (m *scrapeMetrics) noteQueryError(err error, fatal bool) {
	m.queryErrors++
	if fatal {
		m.status = classifyCollectorError(err)
	}
}

func (m *scrapeMetrics) noteParseError(fatal bool) {
	m.parseErrors++
	if fatal {
		m.status = collectorStatusParseError
	}
}

func (m *scrapeMetrics) tryDeepQuery(limit int) bool {
	if limit > 0 && m.deepQueriesAttempted >= int64(limit) {
		m.deepQueriesSkipped++
		return false
	}
	m.deepQueriesAttempted++
	return true
}

func (m *scrapeMetrics) noteDeepQueryAttempt() {
	m.deepQueriesAttempted++
}

func (m *scrapeMetrics) noteDeepQueryError() {
	m.deepQueryErrors++
	m.deepQueriesFailed++
}

func (m *scrapeMetrics) finish(start time.Time) {
	m.scrapeDurationMs = time.Since(start).Milliseconds()
	if m.scrapeDurationMs < 0 {
		m.scrapeDurationMs = 0
	}
}

func (m scrapeMetrics) toMap() map[string]int64 {
	return map[string]int64{
		"collector_status_ok":               boolToInt(m.status == collectorStatusOK),
		"collector_status_permission_error": boolToInt(m.status == collectorStatusPermissionError),
		"collector_status_timeout":          boolToInt(m.status == collectorStatusTimeout),
		"collector_status_query_error":      boolToInt(m.status == collectorStatusQueryError),
		"collector_status_parse_error":      boolToInt(m.status == collectorStatusParseError),
		"collector_scrape_duration_ms":      m.scrapeDurationMs,
		"collector_failures_query":          m.queryErrors,
		"collector_failures_parse":          m.parseErrors,
		"collector_failures_deep_query":     m.deepQueryErrors,
		"collector_deep_queries_attempted":  m.deepQueriesAttempted,
		"collector_deep_queries_failed":     m.deepQueriesFailed,
		"collector_deep_queries_skipped":    m.deepQueriesSkipped,
	}
}

func classifyCollectorError(err error) string {
	switch {
	case err == nil:
		return collectorStatusOK
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return collectorStatusPermissionError
	default:
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return collectorStatusTimeout
		}
		return collectorStatusQueryError
	}
}

func boolToInt(ok bool) int64 {
	if ok {
		return 1
	}
	return 0
}

func isUnsupportedProbeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errFeatureUnsupported) {
		return true
	}

	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "not supported") || strings.Contains(text, "unsupported")
}
