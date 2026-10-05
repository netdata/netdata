// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestDecodedObservationsPreservedAcrossOTLPLogsAndTraces(t *testing.T) {
	const raw = `{
 "meta":{
  "page":{"url":"https://url-user:url-password@example.org/orders/123456?private-query=1#private-fragment"},
  "view":{"name":"/checkout/capture-secret?private-query=1#private-fragment"},
  "session":{"id":"session-one"}, "user":{"id":"internal-user-only"},
  "browser":{"name":"Browser capture-secret","version":"1 capture-secret","os":"OS capture-secret"},
  "app":{"version":"release capture-secret","environment":"env capture-secret"}
 },
 "exceptions":[{"type":"TypeError capture-secret","value":"failed capture-secret","stacktrace":{"frames":[
  {"filename":"https://url-user:url-password@example.org/app.js?private-query=1#private-fragment","function":"render capture-secret","lineno":10,"colno":2}
 ]}}],
 "events":[
  {"name":"session_start"},{"name":"session_resume"},{"name":"session_extend"},
  {"name":"view_changed","attributes":{"fromView":"/checkout/123456?private-query=1","toView":"/checkout/654321#private-fragment"}},
  {"name":"milestone capture-secret","attributes":{"detail":"capture-secret","url.full":"https://url-user:url-password@example.org/api/123456?private-query=1#private-fragment"}}
 ],
 "traces":{"resourceSpans":[{
  "resource":{"attributes":[{"key":"service.name","value":{"stringValue":"service capture-secret"}}]},
  "scopeSpans":[{"scope":{"name":"scope capture-secret","version":"version capture-secret"},"spans":[{
   "traceId":"360b7dd27710ac98272283e6f1f29102","spanId":"17c20d0ec32b813e","name":"request capture-secret","kind":3,
   "startTimeUnixNano":"1","endTimeUnixNano":"2",
   "attributes":[{"key":"url.full","value":{"stringValue":"https://url-user:url-password@example.org/api/123456?private-query=1#private-fragment"}},
    {"key":"detail","value":{"stringValue":"capture-secret"}}, {"key":"http.response.status_code","value":{"intValue":"503"}}],
   "status":{"code":2,"message":"failed capture-secret"}
  }]}]
 }]}}
 `
	opt := faro.Options{
		Site:      "s1",
		Now:       t0,
		Country:   "GR",
		City:      "live-city-only",
		HasGeo:    true,
		EventLogs: true,
		Tracing:   true,
		Redactor:  newRedactor(t, "capture-secret"),
	}
	b, err := faro.Decode([]byte(raw), opt)
	require.NoError(t, err)
	require.Len(t, b.Errors, 1)
	require.Len(t, b.Spans, 1)
	assert.Equal(t, "/orders/:id", b.Path)
	assert.Equal(t, "internal-user-only", b.UserID)
	changed, err := faro.Decode([]byte(strings.ReplaceAll(raw, "private-query=1#private-fragment", "different-query=2#different-fragment")), opt)
	require.NoError(t, err)
	require.Len(t, changed.Errors, 1)
	assert.Equal(t, b.Errors[0].Fingerprint, changed.Errors[0].Fingerprint, "URL query and fragment do not change normalized error identity")

	remoteLogs := &fakeLogsService{}
	// The transport guard is for diagnostics, never a second beacon normalization pass.
	logs := newExporter(t, startServer(t, remoteLogs), newRecCounters(), newRedactor(t, "render"))
	records := logs.build(b, true)
	require.Len(t, records, 4, "page, error, view transition, and milestone; no session lifecycle logs")
	attrs := simplify(records[1]).attrs
	assert.Equal(t, b.Errors[0].Message, attrs["error.message"])
	assert.Equal(t, b.Errors[0].Stack, attrs["error.stack"])
	assert.Equal(t, b.Errors[0].Fingerprint, attrs["error.fingerprint"])
	assert.Equal(t, b.View, attrs["page.view"])
	assert.Equal(t, b.Browser, attrs["browser.name"])
	assert.Equal(t, b.AppVersion, attrs["app.version"])
	assert.Equal(t, "view_changed", simplify(records[2]).attrs["event.name"])
	assert.Equal(t, "https://example.org/api/:id", simplify(records[3]).attrs["event.attr.url.full"])
	require.True(t, logs.export(context.Background(), records, time.Second))
	requests := remoteLogs.requests()
	require.Len(t, requests, 1)

	remoteTraces := &fakeTraceService{}
	traces := newTraceExporter(t, startTraceServer(t, remoteTraces), "s1", newRecCounters(), newRedactor(t, "failed"))
	resource := traces.resourceSpans(b)
	require.NotNil(t, resource)
	require.Len(t, resource.ScopeSpans, 1)
	scope := resource.ScopeSpans[0]
	assert.Equal(t, b.Spans[0].Scope, scope.Scope.Name)
	assert.Equal(t, b.Spans[0].ScopeVersion, scope.Scope.Version)
	require.Len(t, scope.Spans, 1)
	span := scope.Spans[0]
	assert.Equal(t, b.Spans[0].StatusMessage, span.Status.Message)
	assert.Equal(t, b.Spans[0].Name, span.Name)
	require.True(t, traces.exportSpans(context.Background(), []spanItem{{n: 1, rs: resource}}, time.Second))
	require.Equal(t, 1, remoteTraces.count())
	remoteTraces.mu.Lock()
	traceRequest := remoteTraces.reqs[0]
	remoteTraces.mu.Unlock()
	for _, payload := range []string{protojson.Format(requests[0]), protojson.Format(traceRequest)} {
		for _, removed := range []string{"capture-secret", "url-user", "url-password", "private-query", "private-fragment", "internal-user-only", "live-city-only", "session_start", "session_resume", "session_extend"} {
			assert.NotContains(t, payload, removed)
		}
		assert.Contains(t, payload, "[REDACTED]")
	}
}
