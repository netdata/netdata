// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type benchmarkSink struct{}

func (benchmarkSink) Ingest(*beacon.Beacon) {}
func (benchmarkSink) Reject(string, string) {}

type benchmarkResponseWriter struct{ http.ResponseWriter }

func (w benchmarkResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Use the same wrapper interface as the native receiver while measuring the
// routed normalization path without socket or downstream export costs.
func BenchmarkCollectWrapped(b *testing.B) {
	cfg := testCfg()
	cfg.RateLimit.PerIPPerMin = math.MaxInt32
	cfg.RateLimit.PerSitePerSec = math.MaxInt32
	handler := newFixture(cfg, benchmarkSink{}, nil).Handler()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(faroBody))
		req.Header.Set("Origin", "https://shop.example.com")
		req.Header.Set("User-Agent", browserUA)
		response := httptest.NewRecorder()
		handler.ServeHTTP(benchmarkResponseWriter{response}, req)
		if response.Code != http.StatusAccepted {
			b.Fatalf("unexpected response: %d", response.Code)
		}
	}
}
