// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservationIdentityNormalization(t *testing.T) {
	for _, tc := range []struct {
		name, id, revision     string
		validID, validRevision bool
	}{
		{"valid", "v5-1234567890123-1234567890123", "42", true, true},
		{"boundary", strings.Repeat("x", 128), "9007199254740991", true, true},
		{"missing", "", "", false, false},
		{"noncanonical revision", "metric", "01", true, false},
		{"unbounded revision", "metric", strings.Repeat("0", 1024) + "1", true, false},
		{"long", strings.Repeat("x", 129), "9007199254740992", false, false},
		{"control", "metric\n1", "-1", false, false},
		{"opaque URL", "https://example.org", "1.5", false, false},
		{"unicode", "metric☃", "0", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{
				"meta":         map[string]any{"session": map[string]string{"id": tc.id}, "page": map[string]string{"id": tc.id, "url": "https://example.org/entry"}, "view": map[string]string{"id": tc.id, "name": "checkout"}},
				"measurements": []any{map[string]any{"values": map[string]float64{"cls": 0}, "context": map[string]string{"id": tc.id, "observation_sequence": tc.revision}}},
				"events":       []any{map[string]any{"name": "document_activated", "attributes": map[string]string{"observation_id": tc.id, "observation_sequence": tc.revision}}},
			})
			require.NoError(t, err)
			b, err := Decode(raw, Options{
				Now: now,
			})
			require.NoError(t, err)
			require.Len(t, b.Vitals, 1)
			require.Len(t, b.Events, 1)
			assert.Equal(t, "/entry", b.PageGroup)
			assert.Equal(t, "checkout", b.View)
			assert.Equal(t, tc.validID, b.SessionID != "")
			assert.Equal(t, tc.validID, b.ExperienceID != "")
			assert.Equal(t, tc.validID, b.ViewID != "")
			assert.Equal(t, tc.validID, b.Vitals[0].ID != "")
			assert.Equal(t, tc.validID, b.Events[0].ID != "")
			assert.Equal(t, tc.validRevision, b.Vitals[0].Revision != 0)
			assert.Equal(t, tc.validRevision, b.Events[0].Revision != 0)
			assert.Equal(t, beacon.EventDocument, b.Events[0].Kind)
		})
	}
}

func TestCaptureNormalizesBeforeFingerprinting(t *testing.T) {
	opt := Options{Site: "shop", Now: now, EventLogs: true, FrustrationSignals: true,
		Redactor:  redact.NewRedactor("configured-value"),
		PathRules: []PathRule{{Re: regexp.MustCompile(`/members/[^/]+`), Replace: "/members/:member"}},
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
  {"name":"custom","attributes":{"context":"value","observation_id":"not an id"}},
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
	// Observation identity is protocol data only for identified event kinds.
	assert.Equal(t, "not an id", enabled.Events[2].Attrs["observation_id"])
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

// Both performance events name a URL. A resource name identifies the snippet's
// own requests, so it is native; a navigation name is optional diagnostics.
func TestCapturePerformanceEventNamesAreStructuredURLs(t *testing.T) {
	for name, native := range map[string]bool{navigationEvent: false, resourceEvent: true} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"events": []any{map[string]any{
				"name": name, "attributes": map[string]string{"name": "page.html?private=value#fragment"},
			}}})
			require.NoError(t, err)
			for _, logs := range []bool{false, true} {
				b, err := Decode(raw, Options{Now: now, EventLogs: logs})
				require.NoError(t, err)
				require.Len(t, b.Events, 1)
				if native || logs {
					assert.Equal(t, map[string]string{"name": "page.html"}, b.Events[0].Attrs, "event logs %v", logs)
				} else {
					assert.Empty(t, b.Events[0].Attrs)
				}
			}
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
	for _, value := range []string{"-1", "+Inf", "-Inf", "NaN", "Infinity", "1e1000", "I\tnf"} {
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
			assert.Equal(t, []beacon.Resource{{Host: "example.org"}, {Host: "example.org", HasDuration: true, DurationMS: 25}}, b.Resources)
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

// A URL view is reduced like a page URL before credentials are redacted, so a
// credential cannot hide the URL shape and the path rules that follow it.
func TestCaptureURLViewsReduceBeforeRedaction(t *testing.T) {
	for view, want := range map[string]string{
		"token=a/b&email=user@example.com":            "/token=[REDACTED]",
		"/reset/abcdef123456ghijkl789.token=xyz":      "/reset/:token",
		"/files/1a2b3c4d5e6f7g8h9i0jconfigured-value": "/files/:token",
		"/users/jane@example.com?token=abc":           "/users/:email",
	} {
		t.Run(view, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"meta": map[string]any{"view": map[string]string{"name": view}}})
			require.NoError(t, err)
			b, err := Decode(raw, Options{Now: now, Redactor: redact.NewRedactor("configured-value")})
			require.NoError(t, err)
			assert.Equal(t, want, b.View)
		})
	}
}

func TestCapturePreservesHashedScriptIdentity(t *testing.T) {
	opt := Options{
		Now:         now,
		ConsoleLogs: true,
		PathRules:   []PathRule{{Re: regexp.MustCompile(`/members/[^/]+`), Replace: "/members/:member"}},
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

func TestCaptureRedactsCredentialsJoinedByCleaning(t *testing.T) {
	const pem = "-----BEGIN PRIVATE KEY-----MIIBOgIBAAJBAKj34GkxFhD90vcNLYLInFEX6Ppy1tPf-----END PRIVATE KEY-----"
	bound := strings.Repeat("x", maxUserIDLen-14) + " "
	for name, tc := range map[string]struct {
		raw    string
		secret string // the credential value; it must not survive anywhere
		want   string // the normalized user ID
	}{
		"bearer split by NUL":          {raw: "Bea\x00rer abcdef.ghi", secret: "abcdef.ghi", want: "[REDACTED]"},
		"bearer split by escape":       {raw: "Bea\x1brer abcdef.ghi", secret: "abcdef.ghi", want: "[REDACTED]"},
		"bearer split by DEL":          {raw: "Bea\x7frer abcdef.ghi", secret: "abcdef.ghi", want: "[REDACTED]"},
		"bearer split by C1 control":   {raw: "Bea\u0085rer abcdef.ghi", secret: "abcdef.ghi", want: "[REDACTED]"},
		"key split by NUL":             {raw: "tok\x00en=hunter2", secret: "hunter2", want: "token=[REDACTED]"},
		"key split by newline":         {raw: "tok\nen=hunter2", secret: "hunter2", want: "token=[REDACTED]"},
		"access key split by tab":      {raw: "AKIA\tABCDEFGHIJKLMNOP", secret: "ABCDEFGHIJKLMNOP", want: "[REDACTED]"},
		"PEM label split by NUL":       {raw: strings.Replace(pem, "PRIVATE KEY", "PRIVATE\x00 KEY", 1), secret: "MIIBOgIBAAJBAKj34Gkx", want: "[REDACTED]"},
		"configured value split by C1": {raw: "configu\u009fred-value", secret: "configured-value", want: "[REDACTED]"},
		// A control character inside the value ends the match in the received
		// text; the cleaned text holds the whole credential.
		"bearer value split by NUL":              {raw: "Bearer abcdef\x00ghijkl", secret: "ghijkl", want: "[REDACTED]"},
		"configured value after a bearer prefix": {raw: "Bearer configured\t-value", secret: "-value", want: "[REDACTED]"},
		"both shapes in one value":               {raw: "Bearer\tabcdef tok\x00en=hunter2", secret: "hunter2", want: "[REDACTED] token=[REDACTED]"},
		// Shapes the received text already exposes stay redacted: the
		// whitespace and word boundary they need are control characters.
		"bearer after a tab":           {raw: "Bearer\tabcdef.ghi", secret: "abcdef.ghi", want: "[REDACTED]"},
		"key after a control boundary": {raw: "a\x01token=hunter2", secret: "hunter2", want: "atoken=[REDACTED]"},
		// The bound applies to the redacted text, so the value cannot keep a
		// prefix and the result stays within the limit.
		"credential at the bound": {raw: bound + "tok\x00en=hunter2", secret: "hun", want: bound + "token=[REDACT"},
	} {
		t.Run(name, func(t *testing.T) {
			frame := map[string]any{"function": tc.raw, "filename": "https://example.org/app.js", "lineno": 1, "colno": 2}
			serialized, err := json.Marshal(frame)
			require.NoError(t, err)
			text := map[string]any{"stringValue": tc.raw}
			raw, err := json.Marshal(map[string]any{
				"meta": map[string]any{
					"session": map[string]string{"id": "session"},
					"page":    map[string]string{"url": "https://example.org/checkout"},
					"browser": map[string]string{"name": tc.raw, "version": tc.raw, "os": tc.raw},
					"view":    map[string]string{"name": tc.raw},
					"app":     map[string]string{"version": tc.raw, "environment": tc.raw},
					"user":    map[string]string{"id": tc.raw},
				},
				"measurements": []any{map[string]any{"values": map[string]float64{"lcp": 5000}, "context": map[string]string{"element": tc.raw}}},
				"exceptions":   []any{map[string]any{"type": tc.raw, "value": tc.raw, "stacktrace": map[string]any{"frames": []any{frame}}}},
				"events": []any{
					map[string]any{"name": tc.raw, "domain": tc.raw, "attributes": map[string]string{tc.raw: tc.raw, "target": tc.raw}},
					map[string]any{"name": "rage_click", "attributes": map[string]string{"target": tc.raw}},
				},
				"logs": []any{map[string]any{"level": tc.raw, "message": tc.raw, "context": map[string]string{"type": tc.raw, "stackFrames": string(serialized)}}},
				"traces": map[string]any{"resourceSpans": []any{map[string]any{
					"resource": map[string]any{"attributes": []any{map[string]any{"key": "service.name", "value": text}}},
					"scopeSpans": []any{map[string]any{
						"scope": map[string]string{"name": tc.raw, "version": tc.raw},
						"spans": []any{map[string]any{
							"traceId": "1234567890abcdef1234567890abcdef", "spanId": "1234567890abcdef",
							"startTimeUnixNano": "1", "endTimeUnixNano": "2", "name": tc.raw,
							"status":     map[string]any{"message": tc.raw},
							"attributes": []any{map[string]any{"key": tc.raw, "value": text}},
						}},
					}},
				}}},
			})
			require.NoError(t, err)
			b, err := Decode(raw, Options{
				Now:                now,
				EventLogs:          true,
				FrustrationSignals: true,
				ConsoleLogs:        true,
				Tracing:            true,
				Redactor:           redact.NewRedactor("configured-value"),
			})
			require.NoError(t, err)

			assert.Equal(t, tc.want, b.UserID)
			require.Len(t, b.Vitals, 1)
			require.Len(t, b.Errors, 1)
			require.Len(t, b.Events, 2)
			require.Len(t, b.Logs, 1)
			require.Len(t, b.Spans, 1)
			for _, value := range beaconStrings(reflect.ValueOf(b)) {
				assert.NotContains(t, value, tc.secret)
			}
		})
	}
}

func TestConsoleLevelRedactsBeforeLowercasing(t *testing.T) {
	for level, want := range map[string]string{
		"WARN":      "warn",
		" error ":   "error",
		"Lv1Secret": "[REDACTED]",
		"Custom":    "Custom",
	} {
		t.Run(level, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"logs": []any{map[string]string{"level": level, "message": "failed"}}})
			require.NoError(t, err)
			b, err := Decode(raw, Options{Now: now, ConsoleLogs: true, Redactor: redact.NewRedactor("Lv1Secret")})
			require.NoError(t, err)
			require.Len(t, b.Logs, 1)
			assert.Equal(t, want, b.Logs[0].Level)
		})
	}
}

// beaconStrings returns every string a decoded beacon holds, map keys included.
func beaconStrings(v reflect.Value) []string {
	switch v.Kind() {
	case reflect.String:
		return []string{v.String()}
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return beaconStrings(v.Elem())
	case reflect.Slice, reflect.Array:
		var out []string
		for i := range v.Len() {
			out = append(out, beaconStrings(v.Index(i))...)
		}
		return out
	case reflect.Map:
		var out []string
		for iter := v.MapRange(); iter.Next(); {
			out = append(out, beaconStrings(iter.Key())...)
			out = append(out, beaconStrings(iter.Value())...)
		}
		return out
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[time.Time]() {
			return nil
		}
		var out []string
		for i := range v.NumField() {
			out = append(out, beaconStrings(v.Field(i))...)
		}
		return out
	default:
		return nil
	}
}
