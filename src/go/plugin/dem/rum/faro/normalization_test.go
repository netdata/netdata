// SPDX-License-Identifier: GPL-3.0-or-later
package faro

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureNormalizesBeforeFingerprinting(t *testing.T) {
	opt := Options{Site: "shop", Now: now, EventLogs: true, FrustrationSignals: true,
		Redactor:  redact.NewRedactor("configured-value"),
		PathRules: []beacon.PathRule{{Re: regexp.MustCompile(`/members/[^/]+`), Replace: "/members/:member"}},
	}
	var fingerprints []string
	for _, query := range []string{"?request=one#first", "?request=two#second"} {
		raw := `{"meta":{"session":{"id":"session"},"page":{"url":"https://user:password@shop.example.org/members/customer` + query + `"},"view":{"name":"/members/customer` + query + `"},"user":{"id":"123e4567-e89b-12d3-a456-426614174000"},"app":{"version":"configured-value"}},"exceptions":[{"type":"Error","value":"request failed token=variable-secret","stacktrace":{"frames":[{"function":"configured-value","filename":"https://user:password@shop.example.org/members/customer/app.js` + query + `","lineno":12,"colno":3}]}}],"events":[{"name":"custom","attributes":{"url.full":"/members/customer` + query + `","context":"configured-value"}}]}`
		b, err := Decode([]byte(raw), opt)
		require.NoError(t, err)
		assert.Equal(t, "/members/:member", b.Path)
		assert.Equal(t, "shop.example.org", b.PageHost)
		assert.Equal(t, b.Path, b.View)
		assert.Equal(t, b.View, b.PageGroup)
		assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", b.UserID)
		assert.Equal(t, "[REDACTED]", b.AppVersion)
		require.Len(t, b.Errors, 1)
		assert.Equal(t, "request failed token=[REDACTED]", b.Errors[0].Message)
		assert.Equal(t, "[REDACTED] (https://shop.example.org/members/:member/app.js:12:3)\n", b.Errors[0].Stack)
		assert.Equal(t, "/members/:member", b.Events[0].Attrs["url.full"])
		assert.Equal(t, "[REDACTED]", b.Events[0].Attrs["context"])
		fingerprints = append(fingerprints, b.Errors[0].Fingerprint)
	}
	assert.Equal(t, fingerprints[0], fingerprints[1], "query-only source changes must not split the same failure")
}

func TestCaptureDiscardsUnusedAttributesButKeepsNativeEvidence(t *testing.T) {
	raw := []byte(`{"events":[
  {"name":"session_start","attributes":{"unused":"value"}},
  {"name":"view_changed","attributes":{"fromView":"/users/123456","toView":"/users/987654","unused":"value"}},
  {"name":"custom","attributes":{"context":"value"}},
  {"name":"faro.performance.navigation","attributes":{"pageLoadTime":"100","domContentLoadHandlerTime":"10","unused":"value"}},
  {"name":"faro.performance.resource","attributes":{"name":"https://example.org/a?private=value","httpHost":"example.org","duration":"25","transferSize":"500","initiatorType":"fetch","unused":"value"}},
  {"name":"faro.tracing.fetch","attributes":{"url.full":"/api?private=value","http.request.method":"GET","http.response.status_code":"200","duration_ns":"25000000","unused":"value"},"trace":{"trace_id":"1234567890abcdef1234567890abcdef"}},
  {"name":"rage_click","attributes":{"target":"button#123456"}},
  {"name":"securitypolicyviolation","attributes":{"sample":"unused"}},
  {"name":"faro.user.action","attributes":{"userActionName":"unused"}}
 ]}`)
	b, err := Decode(raw, Options{Now: now})
	require.NoError(t, err)
	require.Len(t, b.Events, 6)
	assert.Equal(t, beacon.EventSession, b.Events[0].Kind)
	assert.Empty(t, b.Events[0].Attrs)
	assert.Equal(t, beacon.EventView, b.Events[1].Kind)
	assert.Equal(t, map[string]string{"fromView": "/users/:id", "toView": "/users/:id"}, b.Events[1].Attrs)
	assert.Empty(t, b.Events[2].Attrs)
	require.NotNil(t, b.Navigation)
	assert.Equal(t, float64(100), b.Navigation.LoadMS)
	assert.Equal(t, float64(10), b.Navigation.DCLMS)
	require.Len(t, b.Resources, 1)
	assert.Equal(t, float64(25), b.Resources[0].DurationMS)
	assert.Equal(t, float64(500), b.Resources[0].TransferB)
	assert.Equal(t, "/api", b.Events[5].Attrs["url.full"])
	assert.Equal(t, "1234567890abcdef1234567890abcdef", b.Events[5].TraceID)
	for _, event := range b.Events {
		assert.NotContains(t, event.Attrs, "unused")
	}
	enabled, err := Decode(raw, Options{Now: now, EventLogs: true, FrustrationSignals: true})
	require.NoError(t, err)
	require.Len(t, enabled.Events, 7)
	assert.Equal(t, "value", enabled.Events[2].Attrs["context"])
	assert.Equal(t, "button#:id", enabled.Events[6].Attrs["target"])
}

func TestCaptureRedactsBeforeBounds(t *testing.T) {
	// The credential crosses the type's boundary. Truncating it first would leave
	// a credential prefix that the configured-value matcher could not recognize.
	typ := strings.Repeat("x", maxNameLen-3) + "configured-value"
	raw, err := json.Marshal(map[string]any{"exceptions": []any{map[string]any{"type": typ, "value": "failed"}}, "events": []any{map[string]any{"name": typ}}})
	require.NoError(t, err)
	b, err := Decode(raw, Options{Now: now, Redactor: redact.NewRedactor("configured-value")})
	require.NoError(t, err)
	require.Len(t, b.Errors, 1)
	assert.Equal(t, strings.Repeat("x", maxNameLen-3)+"[RE", b.Errors[0].Type)
	require.Len(t, b.Events, 1)
	assert.Equal(t, strings.Repeat("x", maxNameLen-3)+"[RE", b.Events[0].Name)
}

func TestCaptureSpanAttributeSemanticsPrecedeKeyRedaction(t *testing.T) {
	raw := []byte(`{"traces":{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"1234567890abcdef1234567890abcdef","spanId":"1234567890abcdef","startTimeUnixNano":"1","endTimeUnixNano":"2","attributes":[{"key":"url.full","value":{"stringValue":"https://user:password@example.org/api?private=value#fragment"}}]}]}]}]}}`)
	b, err := Decode(raw, Options{Now: now, Tracing: true, Redactor: redact.NewRedactor("url.full")})
	require.NoError(t, err)
	require.Len(t, b.Spans, 1)
	require.Len(t, b.Spans[0].Attrs, 1)
	assert.Equal(t, "https://example.org/api", b.Spans[0].Attrs[0].Str)
}

func TestCapturePerformanceEventNamesAreStructuredURLs(t *testing.T) {
	for _, name := range []string{navigationEvent, resourceEvent} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"events": []any{map[string]any{
				"name": name, "attributes": map[string]string{"name": "page.html?private=value#fragment"},
			}}})
			require.NoError(t, err)
			b, err := Decode(raw, Options{Now: now, EventLogs: true})
			require.NoError(t, err)
			require.Len(t, b.Events, 1)
			assert.Equal(t, "page.html", b.Events[0].Attrs["name"])
		})
	}
}

func TestCaptureAttributeKeysNormalizeBeforeSemantics(t *testing.T) {
	for _, tc := range []struct{ key, wantKey string }{
		{" url.full ", "url.full"},
		{"http.\turl", "http.url"},
		{"\nurl\r", "url"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			encodedKey, err := json.Marshal(tc.key)
			require.NoError(t, err)
			key := string(encodedKey)
			raw := []byte(`{
 "events":[{"name":"custom","attributes":{` + key + `:"https://user:password@example.org/api?private=value#fragment"}}],
 "traces":{"resourceSpans":[{"scopeSpans":[{"spans":[{
  "traceId":"1234567890abcdef1234567890abcdef","spanId":"1234567890abcdef",
  "startTimeUnixNano":"1","endTimeUnixNano":"2",
  "attributes":[{"key":` + key + `,"value":{"stringValue":"https://user:password@example.org/api?private=value#fragment"}}]
 }]}]}]}}`)
			b, err := Decode(raw, Options{Now: now, Tracing: true, EventLogs: true})
			require.NoError(t, err)
			require.Len(t, b.Spans, 1)
			require.Len(t, b.Spans[0].Attrs, 1)
			assert.Equal(t, "https://example.org/api", b.Spans[0].Attrs[0].Str)
			require.Len(t, b.Events, 1)
			assert.Equal(t, map[string]string{tc.wantKey: "https://example.org/api"}, b.Events[0].Attrs)
		})
	}
}

func TestCaptureRejectsNonfiniteTimingValues(t *testing.T) {
	for _, value := range []string{"+Inf", "-Inf", "NaN", "Infinity", "1e1000", "I\tnf"} {
		t.Run(value, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"events": []any{
				map[string]any{"name": navigationEvent, "attributes": map[string]string{"pageLoadTime": value, "domContentLoadHandlerTime": "10"}},
				map[string]any{"name": resourceEvent, "attributes": map[string]string{"duration": value, "httpHost": "example.org"}},
				map[string]any{"name": resourceEvent, "attributes": map[string]string{"duration": "25", "transferSize": value, "httpHost": "example.org"}},
				map[string]any{"name": "faro.tracing.fetch", "attributes": map[string]string{"duration_ns": value, "http.response.status_code": value}},
			}})
			require.NoError(t, err)
			b, err := Decode(raw, Options{Now: now, EventLogs: true})
			require.NoError(t, err)
			require.NotNil(t, b.Navigation)
			assert.False(t, b.Navigation.HasLoad)
			assert.True(t, b.Navigation.HasDCL)
			assert.Equal(t, float64(10), b.Navigation.DCLMS)
			assert.Equal(t, []beacon.Resource{{Host: "example.org", DurationMS: 25}}, b.Resources)
			require.Len(t, b.Events, 4)
			assert.NotContains(t, b.Events[0].Attrs, "pageLoadTime")
			assert.NotContains(t, b.Events[1].Attrs, "duration")
			assert.NotContains(t, b.Events[2].Attrs, "transferSize")
			assert.Empty(t, b.Events[3].Attrs)
		})
	}
}

func TestCaptureURLPercentAndOpaqueViewHandling(t *testing.T) {
	for _, tc := range []struct{ view, want string }{
		{"/sale/50%-off?private=x#fragment", "/sale/50%25-off"},
		{"data:,private", ""}, {"javascript:private", ""}, {" data:text/plain,private ", ""},
		{"da\tta:,private", ""}, {"java\nscript:private", ""},
		{"checkout-view", "checkout-view"}, {"Checkout?logical#label", "Checkout?logical#label"},
	} {
		t.Run(tc.view, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"meta": map[string]any{
				"page": map[string]string{"url": "https://user:password@example.org/sale/50%-off?private=value#fragment"},
				"view": map[string]string{"name": tc.view}},
				"events": []any{map[string]any{"name": "view_changed", "attributes": map[string]string{"fromView": tc.view, "toView": tc.view}}},
			})
			require.NoError(t, err)
			b, err := Decode(raw, Options{Now: now})
			require.NoError(t, err)
			assert.Equal(t, "/sale/50%25-off", b.Path)
			assert.Equal(t, "example.org", b.PageHost)
			assert.Equal(t, tc.want, b.View)
			require.Len(t, b.Events, 1)
			assert.Equal(t, tc.want, b.Events[0].Attrs["fromView"])
			assert.Equal(t, tc.want, b.Events[0].Attrs["toView"])
		})
	}
}

func TestCapturePreservesHashedScriptIdentity(t *testing.T) {
	opt := Options{
		Now:         now,
		ConsoleLogs: true,
		PathRules:   []beacon.PathRule{{Re: regexp.MustCompile(`/members/[^/]+`), Replace: "/members/:member"}},
	}
	var fingerprints []string
	for _, filename := range []string{"framework-2c79e2a64abdb08b.js", "framework-3c79e2a64abdb08b.mjs", "framework-4c79e2a64abdb08b.cjs"} {
		frame := map[string]any{
			"filename": "https://user:password@example.org/members/customer/123456/" + filename + "?private=value#fragment",
			"function": "render",
			"lineno":   12,
			"colno":    3,
		}
		serialized, err := json.Marshal(frame)
		require.NoError(t, err)
		raw, err := json.Marshal(map[string]any{
			"exceptions": []any{map[string]any{"type": "Error", "value": "failed", "stacktrace": map[string]any{"frames": []any{frame}}}},
			"logs":       []any{map[string]any{"level": "error", "message": "failed", "context": map[string]string{"type": "Error", "stackFrames": string(serialized)}}},
		})
		require.NoError(t, err)
		b, err := Decode(raw, opt)
		require.NoError(t, err)
		require.Len(t, b.Errors, 1)
		require.Len(t, b.Logs, 1)
		assert.Equal(t, "render (https://example.org/members/:member/:id/"+filename+":12:3)\n", b.Errors[0].Stack)
		assert.Equal(t, b.Errors[0].Stack, b.Logs[0].Stack)
		fingerprints = append(fingerprints, b.Errors[0].Fingerprint)
	}
	assert.NotEqual(t, fingerprints[0], fingerprints[1])
	assert.NotEqual(t, fingerprints[1], fingerprints[2])
}
