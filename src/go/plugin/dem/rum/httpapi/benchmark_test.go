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
func BenchmarkCollectWrapped(b *testing.B) { benchmarkCollectBody(b, faroBody) }

func BenchmarkCollectTimingEvents(b *testing.B) {
	benchmarkCollectBody(b, `{"meta":{"session":{"id":"session"},"page":{"id":"document-1","url":"https://shop.example.com/"}},"events":[
 {"name":"faro.performance.navigation","attributes":{"observation_id":"navigation-1","observation_sequence":"2","pageLoadTime":"1000","domContentLoadHandlerTime":"10"}},
 {"name":"faro.performance.resource","attributes":{"observation_id":"resource-1","observation_sequence":"3","name":"app.js?private=value#fragment","httpHost":"cdn.example.com","duration":"1000","transferSize":"1000","initiatorType":"fetch"}},
 {"name":"faro.tracing.fetch","attributes":{"url.full":"https://api.example.com/api?private=value#fragment","http.request.method":"POST","http.response.status_code":"200","duration_ns":"1000"}}
 ]}`)
}

func BenchmarkCollectViews(b *testing.B) {
	benchmarkCollectBody(b, `{"meta":{"session":{"id":"session"},"page":{"id":"document-1","url":"https://shop.example.com/"},"view":{"id":"view-2","name":"checkout"}},"events":[{"name":"view_changed","attributes":{"observation_id":"view-2","observation_sequence":"4","fromView":"catalog","toView":"checkout"}}]}`)
}

func BenchmarkCollectErrors(b *testing.B) {
	benchmarkCollectBody(b, `{"meta":{"session":{"id":"session"},"page":{"id":"document-1","url":"https://shop.example.com/"}},"exceptions":[{"type":"TypeError","value":"failed","stacktrace":{"frames":[{"filename":"https://cdn.example.com/static/framework-2c79e2a64abdb08b.js?private=value#fragment","function":"render","lineno":12,"colno":3}]}}]}`)
}

func benchmarkCollectBody(b *testing.B, body string) {
	cfg := testCfg()
	cfg.RateLimit.PerIPPerMin = math.MaxInt32
	cfg.RateLimit.PerSitePerSec = math.MaxInt32
	handler := newFixture(cfg, benchmarkSink{}, nil).Handler()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(body))
		req.Header.Set("Origin", "https://shop.example.com")
		req.Header.Set("User-Agent", browserUA)
		response := httptest.NewRecorder()
		handler.ServeHTTP(benchmarkResponseWriter{response}, req)
		if response.Code != http.StatusAccepted {
			b.Fatalf("unexpected response: %d", response.Code)
		}
	}
}
