// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestBuild(t *testing.T) {
	tests := map[string]struct {
		pageView bool
		beacon   func() *beacon.Beacon
		want     []simpleRec
	}{
		"pageview": {
			pageView: true,
			beacon: func() *beacon.Beacon {
				b := mkBeacon()

				return b
			},
			want: []simpleRec{{
				body: "pageview /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "pageview", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
		"no session id never starts a session": {
			pageView: true,
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.SessionID = ""

				return b
			},
			want: []simpleRec{{
				body: "pageview /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "pageview", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
		"normalized error preserves severity ERROR": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Errors = []beacon.Error{
					{
						Type:    "TypeError",
						Message: "token=[REDACTED] undefined",
						Stack:   "at f (x.js:1:1) [REDACTED]",
						Time:    t0,
					},
				}
				return b
			},
			want: []simpleRec{{
				body: "TypeError: token=[REDACTED] undefined @ /pricing", severity: "ERROR",
				attrs: map[string]string{
					"rum.type": "error", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
					"error.type": "TypeError", "error.message": "token=[REDACTED] undefined", "error.stack": "at f (x.js:1:1) [REDACTED]",
				},
			}},
		},
		"event carries normalized name and attrs": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Events = []beacon.Event{
					{Name: "checkout", Attrs: map[string]string{"coupon": "[REDACTED]"}, Time: t0},
				}
				return b
			},
			want: []simpleRec{{
				body: "event checkout @ /pricing", severity: "INFO",
				attrs: map[string]string{
					"rum.type": "event", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
					"event.name": "checkout", "event.attr.coupon": "[REDACTED]",
				},
			}},
		},
		"app version/environment/view and error fingerprint": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.AppVersion, b.Environment, b.View = "1.2.3", "production", "checkout-view"
				b.Errors = []beacon.Error{{Type: "TypeError", Message: "boom", Fingerprint: "abcdef012345", Time: t0}}
				return b
			},
			want: []simpleRec{
				{
					body: "TypeError: boom @ /pricing", severity: "ERROR",
					attrs: map[string]string{
						"rum.type": "error", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing", "page.view": "checkout-view",
						"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
						"app.version": "1.2.3", "app.environment": "production",
						"error.type": "TypeError", "error.message": "boom", "error.stack": "", "error.fingerprint": "abcdef012345",
					},
				},
			},
		},
		"console log": {
			beacon: func() *beacon.Beacon {
				b := mkBeacon()
				b.Logs = []beacon.Log{{Level: "warn", Message: "slow request", Time: t0}}
				return b
			},
			want: []simpleRec{{
				body: "console warn: slow request", severity: "WARN",
				attrs: map[string]string{
					"rum.type": "console", "console.level": "warn", "session.id": "sess1", "page.path": "/pricing", "page.group": "/pricing",
					"browser.name": "Chrome", "browser.version": "120", "os": "Linux", "device": "desktop", "country": "GR",
				},
			}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := &Logs{
				now: func() time.Time { return t0 },
			}
			var got []simpleRec
			for _, q := range e.build(tc.beacon(), tc.pageView) {
				got = append(got, simplify(q))
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s:\n got  %#v\n want %#v", name, got, tc.want)
			}
		})
	}
}

func TestNoSyntheticSessionStart(t *testing.T) {
	e := &Logs{
		transport: &transport{},
		now:       func() time.Time { return t0 },
	}
	require.Empty(t, e.build(mkBeacon(), false))
}

func TestConsoleSeverityAndErrorDetails(t *testing.T) {
	e := &Logs{
		transport: &transport{
			redactor: newRedactor(t, "secret-value"),
		},
		now: func() time.Time { return t0 },
	}
	for level, want := range map[string]string{"info": "INFO", "warn": "WARN", "error": "ERROR", "debug": "DEBUG", "trace": "TRACE"} {
		frame, err := json.Marshal(
			map[string]any{"function": strings.Repeat("é", 3000) + "secret-value", "filename": "app.js"},
		)
		require.NoError(t, err)
		raw, err := json.Marshal(map[string]any{"logs": []any{map[string]any{
			"level": level, "message": "secret-value",
			"context": map[string]any{"type": "secret-value TypeError", "stackFrames": string(frame)},
		}}})
		require.NoError(t, err)
		b, err := faro.Decode(raw, faro.Options{
			Now:         t0,
			ConsoleLogs: true,
			Redactor:    newRedactor(t, "secret-value"),
		})
		require.NoError(t, err)
		recs := e.build(b, false)
		require.Len(t, recs, 1)
		rec := simplify(recs[0])
		require.Equal(t, want, rec.severity)
		require.NotContains(t, rec.body, "secret-value")
		require.Equal(t, "[REDACTED] TypeError", rec.attrs["error.type"])
		require.LessOrEqual(t, len(rec.attrs["error.stack"]), 4096)
	}
}

func TestConsoleErrorAttributesOnlyWhenPresent(t *testing.T) {
	e := newExporter(t, "127.0.0.1:1", newRecCounters(), nil)
	b := mkBeacon()
	b.Logs = []beacon.Log{{Level: "info", Message: "plain console line"}}
	records := e.build(b, false)
	require.Len(t, records, 1)
	attrs := simplify(records[0]).attrs
	assert.Equal(t, "info", attrs["console.level"])
	assert.NotContains(t, attrs, "error.type")
	assert.NotContains(t, attrs, "error.stack")
}

func TestDecodedConsoleUnicodeRemainsExportable(t *testing.T) {
	for _, field := range []string{"type", "message", "function", "assembled stack"} {
		t.Run(field, func(t *testing.T) {
			typ, message, function := "TypeError", "message", "render"
			filename := "app.js"
			switch field {
			case "type":
				typ = strings.Repeat("界", 30)
			case "message":
				message = strings.Repeat("界", 400)
			case "function":
				function = strings.Repeat("界", 1500)
			case "assembled stack":
				function = strings.Repeat("界", 1365)
			}
			frame, err := json.Marshal(
				map[string]any{"function": function, "filename": filename, "lineno": 1, "colno": 2},
			)
			require.NoError(t, err)
			encoded := string(frame)
			if field == "assembled stack" {
				encoded = `{"function":"render","filename":"app.js","lineno":1,"colno":2} ` + encoded
			}
			raw, err := json.Marshal(map[string]any{"logs": []any{map[string]any{
				"message": message, "level": "error", "context": map[string]any{"type": typ, "stackFrames": encoded},
			}}})
			require.NoError(t, err)
			b, err := faro.Decode(raw, faro.Options{
				Now:         t0,
				ConsoleLogs: true,
			})
			require.NoError(t, err)
			require.Len(t, b.Logs, 1)
			l := b.Logs[0]
			assert.True(t, utf8.ValidString(l.ErrorType))
			assert.True(t, utf8.ValidString(l.Message))
			assert.True(t, utf8.ValidString(l.Stack))
			assert.LessOrEqual(t, len(l.ErrorType), 64)
			assert.LessOrEqual(t, len(l.Message), 1024)
			assert.LessOrEqual(t, len(l.Stack), 4096)
			e := newExporter(t, "127.0.0.1:1", newRecCounters(), nil)
			records := e.build(b, false)
			require.Len(t, records, 1)
			_, err = proto.Marshal(records[0])
			require.NoError(t, err, "one truncated console value must not prevent protobuf encoding")
		})
	}
}

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
		Site: "s1",
		Now:  t0,
		Location: beacon.Location{
			Country: "GR",
			City:    "live-city-only",
			HasGeo:  true,
		},
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
