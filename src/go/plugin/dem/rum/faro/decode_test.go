// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

var now = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

func TestParseFaro(t *testing.T) {
	ts := now.Add(-2 * time.Second)
	tsStr := ts.Format(time.RFC3339Nano)
	tests := map[string]struct {
		body        string
		consoleLogs bool
		want        beacon.Beacon
	}{
		"web-vitals values map, mobile, page group": {
			body: `{"meta":{"page":{"url":"https://x/orders/123e4567-e89b-12d3-a456-426614174000/items?q=1#f","id":"p1"},
			         "session":{"id":"sess"},"browser":{"name":"Safari","version":"17.1","os":"iOS","mobile":true}},
			  "measurements":[{"type":"web-vitals","values":{"lcp":2100,"cls":0.12,"delta":5,"unknown":3},"timestamp":"` + tsStr + `"},
			                  {"type":"web-vitals","values":{"inp":-1}}]}`,
			want: beacon.Beacon{
				Site:           "s",
				Received:       now,
				SessionID:      "sess",
				PageID:         "p1",
				Path:           "/orders/:id/items",
				PageHost:       "x",
				PageGroup:      "/orders/:id/items",
				Browser:        "Safari",
				BrowserVersion: "17.1",
				OS:             "iOS",
				Device:         "mobile",
				Country:        "GR",
				Vitals: []beacon.Vital{
					{Name: "LCP", Value: 2100, Time: ts},
					{Name: "CLS", Value: 0.12, Time: ts},
				},
			},
		},
		"exceptions with and without frames": {
			body: `{"meta":{"page":{"url":"https://x/pricing"}},"exceptions":[
			  {"type":"TypeError","value":"x is undefined","stacktrace":{"frames":[{"filename":"https://x/app.js","function":"render","lineno":10,"colno":5}]},"timestamp":"` + tsStr + `"},
			  {"type":"Error","value":"no stack"}]}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/pricing",
				PageHost:  "x",
				PageGroup: "/pricing",
				Device:    "desktop",
				Country:   "GR",
				Errors: []beacon.Error{
					{Type: "TypeError", Message: "x is undefined", Stack: "render (https://x/app.js:10:5)\n", Time: ts,
						Fingerprint: beacon.Fingerprint("TypeError", "x is undefined", "render@https://x/app.js")},
					{Type: "Error", Message: "no stack", Time: now,
						Fingerprint: beacon.Fingerprint("Error", "no stack", "")},
				},
			},
		},
		"app version/env, spa view, tablet": {
			body: `{"meta":{"page":{"url":"https://shop.example.com/checkout"},"view":{"name":"checkout-view"},
			  "app":{"version":"1.2.3","environment":"production"},
			  "browser":{"name":"Chrome","mobile":true,"viewportWidth":"768"}}}`,
			want: beacon.Beacon{
				Site:        "s",
				Received:    now,
				Browser:     "Chrome",
				Path:        "/checkout",
				PageHost:    "shop.example.com",
				PageGroup:   "checkout-view",
				View:        "checkout-view",
				Device:      "tablet",
				Country:     "GR",
				AppVersion:  "1.2.3",
				Environment: "production",
			},
		},
		"mobile narrow viewport stays mobile": {
			body: `{"meta":{"page":{"url":"https://x/"},"browser":{"mobile":true,"viewportWidth":"390"}}}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageHost:  "x",
				PageGroup: "/",
				Device:    "mobile",
				Country:   "GR",
			},
		},
		"mobile unknown viewport stays mobile": {
			body: `{"meta":{"page":{"url":"https://x/"},"browser":{"mobile":true}}}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageHost:  "x",
				PageGroup: "/",
				Device:    "mobile",
				Country:   "GR",
			},
		},
		"navigation and resource timing events": {
			body: `{"meta":{"page":{"url":"https://x/"}},"events":[
			  {"name":"faro.performance.navigation","attributes":{"pageLoadTime":"1234","domContentLoadHandlerTime":"12","ttfb":"45","type":"navigate"},"timestamp":"` + tsStr + `"},
			  {"name":"faro.performance.resource","attributes":{"httpHost":"cdn.example.com","duration":"88","transferSize":"2048","initiatorType":"script"},"timestamp":"` + tsStr + `"}]}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageHost:  "x",
				PageGroup: "/",
				Device:    "desktop",
				Country:   "GR",
				Events: []beacon.Event{
					{
						Name: "faro.performance.navigation", Kind: beacon.EventNavigation,
						Attrs: map[string]string{
							"pageLoadTime":              "1234",
							"domContentLoadHandlerTime": "12",
							"ttfb":                      "45",
							"type":                      "navigate",
						},
						Time: ts,
					},
					{
						Name: "faro.performance.resource", Kind: beacon.EventResource,
						Attrs: map[string]string{
							"httpHost":      "cdn.example.com",
							"duration":      "88",
							"transferSize":  "2048",
							"initiatorType": "script",
						},
						Time: ts,
					},
				},
				Navigation: &beacon.Navigation{
					LoadMS:  1234,
					HasLoad: true,
					DCLMS:   12,
					HasDCL:  true,
				},
				Resources: []beacon.Resource{
					{Host: "cdn.example.com", DurationMS: 88, TransferB: 2048, Initiator: "script"},
				},
			},
		},
		"the snippet's own collector request is marked self": {
			body: `{"meta":{"page":{"url":"https://x/"}},"events":[
			  {"name":"faro.performance.resource","attributes":{"name":"https://rum.x/rum/s/collect","httpHost":"rum.x","duration":"40","initiatorType":"fetch"},"timestamp":"` + tsStr + `"},
			  {"name":"faro.performance.resource","attributes":{"name":"https://x/api/orders?id=1","httpHost":"x","duration":"75","initiatorType":"fetch"},"timestamp":"` + tsStr + `"}]}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageHost:  "x",
				PageGroup: "/",
				Device:    "desktop",
				Country:   "GR",
				Events: []beacon.Event{
					{
						Name: "faro.performance.resource", Kind: beacon.EventResource,
						Attrs: map[string]string{
							"name":          "https://rum.x/rum/s/collect",
							"httpHost":      "rum.x",
							"duration":      "40",
							"initiatorType": "fetch",
						},
						Time: ts,
					},
					{
						Name: "faro.performance.resource", Kind: beacon.EventResource,
						Attrs: map[string]string{
							"name":          "https://x/api/orders",
							"httpHost":      "x",
							"duration":      "75",
							"initiatorType": "fetch",
						},
						Time: ts,
					},
				},
				Resources: []beacon.Resource{
					{Host: "rum.x", DurationMS: 40, Initiator: "fetch", Self: true},
					{Host: "x", DurationMS: 75, Initiator: "fetch"},
				},
			},
		},
		"events and logs (console off)": {
			body: `{"meta":{"page":{"url":"https://x/"}},
			  "events":[{"name":"session_start","domain":"browser","attributes":{"a":"1"},"timestamp":"` + tsStr + `"},{"name":""}],
			  "logs":[{"message":"hello","level":"info"}]}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageHost:  "x",
				PageGroup: "/",
				Device:    "desktop",
				Country:   "GR",
				Events: []beacon.Event{
					{Kind: beacon.EventSession, Name: "session_start", Domain: "browser", Attrs: map[string]string{"a": "1"}, Time: ts},
				},
			},
		},
		"logs collected when enabled": {
			body:        `{"meta":{"page":{"url":"https://x/"}},"logs":[{"message":"hello","level":"WARN","timestamp":"` + tsStr + `"},{"message":""}]}`,
			consoleLogs: true,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageHost:  "x",
				PageGroup: "/",
				Device:    "desktop",
				Country:   "GR",
				Logs:      []beacon.Log{{Level: "warn", Message: "hello", Time: ts}},
			},
		},
		"empty object": {
			body: `{}`,
			want: beacon.Beacon{
				Site:      "s",
				Received:  now,
				Path:      "/",
				PageGroup: "/",
				Device:    "desktop",
				Country:   "GR",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Decode(
				[]byte(tc.body),
				Options{
					Site:               "s",
					Now:                now,
					Country:            "GR",
					ConsoleLogs:        tc.consoleLogs,
					EventLogs:          true,
					FrustrationSignals: true,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			// The values map iterates in random order: sort vitals by the
			// fixed emission order for a stable comparison.
			sortVitals(got.Vitals)
			sortVitals(tc.want.Vitals)
			if !reflect.DeepEqual(*got, tc.want) {
				t.Fatalf("mismatch:\ngot  %+v\nwant %+v", *got, tc.want)
			}
		})
	}
}

func sortVitals(v []beacon.Vital) {
	order := map[string]int{}
	for i, n := range beacon.Vitals {
		order[n] = i
	}
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && order[v[j].Name] < order[v[j-1].Name]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func TestParseFaroRejectsBadJSON(t *testing.T) {
	for name, body := range map[string]string{"truncated": `{"meta":`, "array": `[]`, "text": `nope`} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(body), Options{
				Site: "s",
				Now:  now,
			}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseFaroBounds(t *testing.T) {
	long := strings.Repeat("z", 10000)
	body := `{"meta":{"browser":{"name":"` + long + `"}},"exceptions":[{"type":"E","value":"` + long + `","stacktrace":{"frames":[{"function":"` + long + `","filename":"app.js"}]}}]}`
	got, err := Decode([]byte(body), Options{
		Site: "s",
		Now:  now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Browser) != maxNameLen || len(got.Errors[0].Message) != maxMessageLen ||
		len(got.Errors[0].Stack) != maxStackLen {
		t.Fatalf(
			"bounds not applied: browser=%d message=%d stack=%d",
			len(got.Browser),
			len(got.Errors[0].Message),
			len(got.Errors[0].Stack),
		)
	}
}

// TestParseFaroSetsGeoFields pins the city/lat/lon/hasGeo pass-through:
// Decode just carries whatever the caller
// resolved for the client IP straight onto the beacon.
func TestParseFaroSetsGeoFields(t *testing.T) {
	got, err := Decode(
		[]byte(`{}`),
		Options{
			Site:    "s",
			Now:     now,
			Country: "GR",
			City:    "Athens",
			Lat:     38.0,
			Lon:     23.7,
			HasGeo:  true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Country != "GR" || got.City != "Athens" || got.Lat != 38.0 || got.Lon != 23.7 || !got.HasGeo {
		t.Fatalf("geo fields = %+v", got)
	}

	got, err = Decode([]byte(`{}`), Options{
		Site: "s",
		Now:  now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.City != "" || got.Lat != 0 || got.Lon != 0 || got.HasGeo {
		t.Fatalf("unresolved geo must stay zero-value: %+v", got)
	}
}

func TestFlexTime(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want time.Time
	}{
		"rfc3339 recent": {raw: `"2030-01-01T11:59:00Z"`, want: time.Date(2030, 1, 1, 11, 59, 0, 0, time.UTC)},
		"rfc3339 millis": {
			raw:  `"2030-01-01T11:59:00.250Z"`,
			want: time.Date(2030, 1, 1, 11, 59, 0, 250e6, time.UTC),
		},
		"unix seconds":         {raw: `1893499140`, want: time.Unix(1893499140, 0)},
		"unix millis":          {raw: `1893499140000`, want: time.Unix(1893499140, 0)},
		"too old collapses":    {raw: `"2020-01-01T00:00:00Z"`, want: now},
		"future collapses":     {raw: `"2030-01-02T00:00:00Z"`, want: now},
		"garbage":              {raw: `"yesterday"`, want: now},
		"empty":                {raw: ``, want: now},
		"slightly future kept": {raw: `"2030-01-01T12:05:00Z"`, want: time.Date(2030, 1, 1, 12, 5, 0, 0, time.UTC)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := flexTime(json.RawMessage(tc.raw), now); !got.Equal(tc.want) {
				t.Fatalf("flexTime(%s) = %v want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// A captured tracing beacon: fetch spans with hex ids, query strings
// stripped from URLs, and the traced-request events carry the trace id.
func TestParseFaroTracingSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/tracing.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	b, err := Decode(raw, Options{
		Site:    "s",
		Now:     now,
		Tracing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Spans) != 2 || b.ServiceName != "trace-test" {
		t.Fatalf("spans = %d, service = %q", len(b.Spans), b.ServiceName)
	}
	sp := b.Spans[0]
	if sp.TraceID != "360b7dd27710ac98272283e6f1f29102" || sp.SpanID != "17c20d0ec32b813e" || sp.Kind != 3 ||
		sp.EndNS <= sp.StartNS {
		t.Fatalf("span = %+v", sp)
	}
	attrs := map[string]beacon.SpanAttr{}
	for _, a := range sp.Attrs {
		attrs[a.Key] = a
	}
	if got := attrs["url.full"].Str; got != "http://127.0.0.1:8801/api/hello" {
		t.Fatalf("url.full = %q, want the query stripped", got)
	}
	if a := attrs["http.response.status_code"]; !a.IsInt || a.Int != 200 {
		t.Fatalf("status code attr = %+v", a)
	}
	var traced int
	for _, e := range b.Events {
		if e.Name == fetchTraceEvent {
			traced++
			if e.Kind != beacon.EventRequest || e.TraceID == "" || strings.Contains(e.Attrs["url.full"], "?") {
				t.Fatalf("traced event = %+v", e)
			}
		}
	}
	if traced != 2 {
		t.Fatalf("traced fetch events = %d", traced)
	}

	off, err := Decode(raw, Options{
		Site: "s",
		Now:  now,
	})
	if err != nil || len(off.Spans) != 0 {
		t.Fatalf("tracing off must drop spans: %d, %v", len(off.Spans), err)
	}
}

// Checks redacted paths (built-in and site rules), the element Faro
// attributes a vital to, the user id, and redacted frustration targets.
func TestParseFaroPrivacyAttributionAndUser(t *testing.T) {
	body := `{"meta":{"page":{"url":"https://shop.example.com/u/jane/orders/jane@example.com?x=1"},"session":{"id":"s1"},"user":{"id":"u_42","email":"jane@example.com"},"browser":{"name":"Chrome"}},
	"measurements":[{"type":"web-vitals","values":{"lcp":5200},"context":{"element":"#hero img.user-1234567"}},{"type":"web-vitals","values":{"inp":620},"context":{"interaction_target":"button#buy"}}],
	"events":[{"name":"rage_click","attributes":{"target":"div#row-jane@example.com > button"}},{"name":"faro.performance.resource","attributes":{"name":"https://cdn.example.com/avatars/jane@example.com.png?v=2"}}]}`
	opt := Options{
		FrustrationSignals: true,
		Site:               "s",
		Now:                time.Now(),
		PathRules:          []beacon.PathRule{{Re: regexp.MustCompile(`^/u/[^/]+`), Replace: "/u/:user"}},
	}
	b, err := Decode([]byte(body), opt)
	if err != nil {
		t.Fatal(err)
	}
	if b.Path != "/u/:user/orders/:email" {
		t.Fatalf("path = %q", b.Path)
	}
	if b.UserID != "u_42" {
		t.Fatalf("user id = %q", b.UserID)
	}
	els := map[string]string{}
	for _, v := range b.Vitals {
		els[v.Name] = v.Element
	}
	if els[beacon.LCP] != "#hero img.user-:id" || els[beacon.INP] != "button#buy" {
		t.Fatalf("elements = %v", els)
	}
	if got := b.Events[0].Attrs["target"]; got != "div#:email > button" {
		t.Fatalf("rage target = %q", got)
	}
	if got := b.Events[1].Attrs["name"]; got != "https://cdn.example.com/avatars/:email" {
		t.Fatalf("resource url = %q", got)
	}
}

func TestIsCollectorRequest(t *testing.T) {
	tests := map[string]bool{
		"https://rum.example.com/rum/shop/collect":       true,
		"https://example.com/edge/rum/shop/collect":      true,
		"https://rum.example.com/rum/shop/collect?bot=1": true,
		"https://example.com/rum/collect":                false,
		"https://example.com/rum//collect":               false,
		"https://example.com/api/orders":                 false,
		"https://example.com/rum/shop.js":                false,
		"https://example.com/rum/shop/collect/extra":     false,
		"":       false,
		"://bad": false,
	}
	for raw, want := range tests {
		if got := isCollectorRequest(raw); got != want {
			t.Errorf("isCollectorRequest(%q) = %v want %v", raw, got, want)
		}
	}
}

func TestDecodeEventKinds(t *testing.T) {
	raw := []byte(
		`{"events":[{"name":"faro.performance.navigation"},{"name":"faro.performance.resource"},{"name":"faro.tracing.fetch"},{"name":"faro.tracing.xml-http-request"},{"name":"custom"}]}`,
	)
	b, err := Decode(raw, Options{
		Site: "s",
		Now:  now,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name string
		kind beacon.EventKind
	}{
		{"faro.performance.navigation", beacon.EventNavigation},
		{"faro.performance.resource", beacon.EventResource},
		{"faro.tracing.fetch", beacon.EventRequest},
		{"faro.tracing.xml-http-request", beacon.EventRequest},
		{"custom", beacon.EventCustom},
	}
	if len(b.Events) != len(want) {
		t.Fatalf("events = %+v", b.Events)
	}
	for i, w := range want {
		if b.Events[i].Name != w.name || b.Events[i].Kind != w.kind {
			t.Errorf("event %d = %+v; want %s/%d", i, b.Events[i], w.name, w.kind)
		}
	}
}
