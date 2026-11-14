// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPingAndRejectedOrigin(t *testing.T) {
	s := newFixture(testCfg(), newRecSink(), nil)
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rum/ping", nil))
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("ping = %d %v", rec.Code, rec.Header())
	}

	req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://www.shop.example.com")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("collect from a foreign origin = %d", rec.Code)
	}
	got := s.route("shop").LastRejectedOrigin()
	if got.Origin != "https://www.shop.example.com" || got.At.IsZero() {
		t.Fatalf("last rejected = %+v", got)
	}
}

func TestCollectOriginPolicy(t *testing.T) {
	tests := map[string]struct {
		hdr        map[string]string
		wantStatus int
		wantACAO   string
		wantReject string
	}{
		"exact origin echoed": {
			hdr: map[string]string{
				"Origin": "https://shop.example.com",
			}, wantStatus: 202, wantACAO: "https://shop.example.com",
		},
		"wildcard subdomain echoed exactly": {
			hdr: map[string]string{
				"Origin": "https://Deep.Sub.example.org",
			}, wantStatus: 202, wantACAO: "https://Deep.Sub.example.org",
		},
		"wildcard never matches the bare domain": {
			hdr: map[string]string{
				"Origin": "https://example.org",
			}, wantStatus: 403, wantReject: "rejected_origin",
		},
		"default port normalized": {
			hdr: map[string]string{
				"Origin": "https://shop.example.com:443",
			}, wantStatus: 202, wantACAO: "https://shop.example.com:443",
		},
		"scheme must match": {
			hdr: map[string]string{
				"Origin": "http://shop.example.com",
			}, wantStatus: 403, wantReject: "rejected_origin",
		},
		"wrong origin": {
			hdr: map[string]string{
				"Origin": "https://evil.example.net",
			}, wantStatus: 403, wantReject: "rejected_origin",
		},
		"missing origin and referer": {
			hdr: map[string]string{}, wantStatus: 403, wantReject: "rejected_origin",
		},
		"null origin": {
			hdr: map[string]string{"Origin": "null"}, wantStatus: 403, wantReject: "rejected_origin",
		},
		"referer fallback": {
			hdr: map[string]string{
				"Referer": "https://shop.example.com/pricing?x=1",
			}, wantStatus: 202, wantACAO: "https://shop.example.com",
		},
		"port in origin matched": {
			hdr: map[string]string{
				"Origin": "http://127.0.0.1:19938",
			}, wantStatus: 202, wantACAO: "http://127.0.0.1:19938",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sink := newRecSink()
			_, ts := newTestServer(t, testCfg(), sink, nil)
			resp := post(t, ts, "/rum/shop/collect", faroBody, tc.hdr)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d want %d", resp.StatusCode, tc.wantStatus)
			}
			acao := resp.Header.Get("Access-Control-Allow-Origin")
			if acao != tc.wantACAO {
				t.Fatalf("ACAO %q want %q", acao, tc.wantACAO)
			}
			if acao == "*" {
				t.Fatal("wildcard ACAO must never be emitted")
			}
			if tc.wantACAO != "" && !strings.Contains(resp.Header.Get("Vary"), "Origin") {
				t.Fatal("Vary: Origin missing")
			}
			n, rejects := sink.snapshot()
			wantRejects := map[string]int{}
			if tc.wantReject != "" {
				wantRejects[tc.wantReject] = 1
			}
			if len(rejects) != len(wantRejects) || (tc.wantReject != "" && rejects[tc.wantReject] != 1) {
				t.Fatalf("rejects %v want %v", rejects, wantRejects)
			}
			if wantN := map[bool]int{true: 1, false: 0}[tc.wantStatus == 202]; n != wantN {
				t.Fatalf("ingested %d want %d", n, wantN)
			}
		})
	}
}

func TestPreflight(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(t, testCfg(), sink, nil)
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/rum/shop/collect", nil)
	req.Header.Set("Origin", "https://shop.example.com")
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-faro-session-id")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", resp.StatusCode)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":  "https://shop.example.com",
		"Access-Control-Allow-Methods": "POST, OPTIONS",
		"Access-Control-Allow-Headers": "content-type,x-faro-session-id",
		"Access-Control-Max-Age":       "86400",
	}
	for k, v := range want {
		if resp.Header.Get(k) != v {
			t.Fatalf("%s = %q want %q", k, resp.Header.Get(k), v)
		}
	}

	req, _ = http.NewRequest(http.MethodOptions, ts.URL+"/rum/shop/collect", nil)
	req.Header.Set("Origin", "https://evil.example.net")
	resp, _ = ts.Client().Do(req)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad-origin preflight status %d", resp.StatusCode)
	}
}

func TestBodyLimits(t *testing.T) {
	origin := map[string]string{"Origin": "https://shop.example.com"}
	tests := map[string]struct {
		body       string
		wantStatus int
		wantReject string
	}{
		"oversize body": {
			body:       `{"meta":{},"pad":"` + strings.Repeat("x", 3000) + `"}`,
			wantStatus: 413,
			wantReject: "rejected_size",
		},
		"exactly at cap": {
			body:       `{"meta":{},"pad":"` + strings.Repeat("x", 2048-len(`{"meta":{},"pad":""}`)) + `"}`,
			wantStatus: 202,
		},
		"one over cap": {
			body:       `{"meta":{},"pad":"` + strings.Repeat("x", 2049-len(`{"meta":{},"pad":""}`)) + `"}`,
			wantStatus: 413,
			wantReject: "rejected_size",
		},
		"bad json":        {body: `{"meta":`, wantStatus: 400, wantReject: "invalid"},
		"not json at all": {body: `hello`, wantStatus: 400, wantReject: "invalid"},
		"empty object ok": {body: `{}`, wantStatus: 202},
		"valid faro":      {body: faroBody, wantStatus: 202},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sink := newRecSink()
			_, ts := newTestServer(t, testCfg(), sink, nil)
			resp := post(t, ts, "/rum/shop/collect", tc.body, origin)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d want %d", resp.StatusCode, tc.wantStatus)
			}
			_, rejects := sink.snapshot()
			if tc.wantReject != "" && rejects[tc.wantReject] != 1 {
				t.Fatalf("rejects %v want %s", rejects, tc.wantReject)
			}
			if tc.wantReject == "" && len(rejects) != 0 {
				t.Fatalf("unexpected rejects %v", rejects)
			}
		})
	}
}

func TestOversizeWithoutContentLength(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(t, testCfg(), sink, nil)
	// Chunked upload: the server cannot pre-check Content-Length and must
	// stop at the cap while reading.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte(`{"pad":"`))
		_, _ = pw.Write(bytes.Repeat([]byte("y"), 4000))
		_, _ = pw.Write([]byte(`"}`))
		_ = pw.Close()
	}()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/rum/shop/collect", pr)
	req.Header.Set("Origin", "https://shop.example.com")
	req.Header.Set("User-Agent", browserUA)
	req.ContentLength = -1
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d want 413", resp.StatusCode)
	}
	_, rejects := sink.snapshot()
	if rejects["rejected_size"] != 1 {
		t.Fatalf("rejects %v", rejects)
	}
}

func TestIngestedBeaconShape(t *testing.T) {
	sink := newRecSink()
	s, ts := newTestServer(t, testCfg(), sink, nil)
	now := time.Date(2030, 1, 1, 0, 0, 5, 0, time.UTC)
	s.now = func() time.Time { return now }
	post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://shop.example.com"})
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ingested) != 1 {
		t.Fatalf("ingested %d", len(sink.ingested))
	}
	got := *sink.ingested[0]
	want := beacon.Beacon{
		Site:           "shop",
		Received:       now,
		SessionID:      "s1",
		ExperienceID:   "document-1",
		Path:           "/users/42",
		PageGroup:      "/users/:id",
		Browser:        "Chrome",
		BrowserVersion: "120",
		OS:             "Linux",
		Device:         "desktop",
		Vitals:         []beacon.Vital{{ID: "vital-1", Revision: 2, Name: "LCP", Value: 1234.5, Time: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}
	if got.Site != want.Site || !got.Received.Equal(want.Received) || got.SessionID != want.SessionID ||
		got.ExperienceID != want.ExperienceID || got.Path != want.Path || got.PageGroup != want.PageGroup || got.Browser != want.Browser ||
		got.BrowserVersion != want.BrowserVersion || got.OS != want.OS || got.Device != want.Device ||
		got.Country != "" || len(got.Vitals) != 1 || got.Vitals[0].Name != "LCP" || got.Vitals[0].Value != 1234.5 ||
		!got.Vitals[0].Time.Equal(want.Vitals[0].Time) || got.Errors != nil || len(got.Events) != 1 || got.Logs != nil {
		t.Fatalf("beacon mismatch:\ngot  %+v\nwant %+v", got, want)
	}
	if got.Vitals[0].ID != "vital-1" || got.Vitals[0].Revision != 2 || got.Events[0].Kind != beacon.EventDocument || got.Events[0].ID != "document-1" || got.Events[0].Revision != 1 {
		t.Fatalf("measurement identity mismatch: %+v", got)
	}
}

const headlessUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36"

// Bots are filtered by default: counted, never ingested.
func TestCollectFiltersBots(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(t, testCfg(), sink, nil)
	origin := "https://shop.example.com"
	for _, hdr := range []map[string]string{
		{"Origin": origin, "User-Agent": headlessUA},
		{"Origin": origin, "User-Agent": "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
	} {
		if resp := post(t, ts, "/rum/shop/collect", faroBody, hdr); resp.StatusCode != 204 {
			t.Fatalf("bot beacon status %d", resp.StatusCode)
		}
	}
	if resp := post(t, ts, "/rum/shop/collect?bot=1", faroBody, map[string]string{"Origin": origin}); resp.StatusCode != 204 {
		t.Fatalf("snippet-flagged bot status %d", resp.StatusCode)
	}
	n, rejects := sink.snapshot()
	if n != 0 || rejects[beacon.RejectBot] != 3 {
		t.Fatalf("ingested %d, rejects %v", n, rejects)
	}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": origin}); resp.StatusCode != 202 {
		t.Fatalf("a real browser must still be accepted: %d", resp.StatusCode)
	}
}

func TestCollectKeepsBotsWhenIncluded(t *testing.T) {
	cfg := testCfg()
	cfg.Sites[0].Bots = config.BotsInclude
	sink := newRecSink()
	_, ts := newTestServer(t, cfg, sink, nil)
	if resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://shop.example.com", "User-Agent": headlessUA}); resp.StatusCode != 202 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ingested) != 1 || sink.ingested[0].Device != beacon.DeviceBot {
		t.Fatalf("ingested %+v", sink.ingested)
	}
}

// fullGeo is a Locator stub carrying city/lat/lon too, for asserting the server → faro.Decode wiring end to end
// (decode_test.go covers faro.Decode's own decoding in isolation).
type fullGeo beacon.Location

func (g fullGeo) Lookup(string) beacon.Location { return beacon.Location(g) }

func TestGeoIPAppliedOnlyForPublicClients(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(t, testCfg(), sink, fixedGeo("GR"))
	// httptest client is 127.0.0.1 (trusted proxy) → XFF sets the client.
	post(
		t,
		ts,
		"/rum/shop/collect",
		faroBody,
		map[string]string{"Origin": "https://shop.example.com", "X-Forwarded-For": "203.0.113.1"},
	)
	post(
		t,
		ts,
		"/rum/shop/collect",
		faroBody,
		map[string]string{"Origin": "https://shop.example.com", "X-Forwarded-For": "192.168.1.5"},
	)
	post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://shop.example.com"})
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ingested) != 3 {
		t.Fatalf("ingested %d", len(sink.ingested))
	}
	want := []string{"GR", "", ""}
	for i, b := range sink.ingested {
		if b.Country != want[i] {
			t.Fatalf("beacon %d country %q want %q", i, b.Country, want[i])
		}
	}
}

func TestGeoCityLatLonAppliedFromResolver(t *testing.T) {
	cfg := testCfg()
	cfg.Sites[0].Capture = &config.Capture{
		Geolocation: new(config.GeolocationCity),
	}
	sink := newRecSink()
	_, ts := newTestServer(
		t,
		cfg,
		sink,
		fullGeo{
			Country: "GR",
			City:    "Athens",
			Lat:     38.0,
			Lon:     23.7,
			HasGeo:  true,
		},
	)
	post(
		t,
		ts,
		"/rum/shop/collect",
		faroBody,
		map[string]string{"Origin": "https://shop.example.com", "X-Forwarded-For": "203.0.113.1"},
	)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ingested) != 1 {
		t.Fatalf("ingested %d", len(sink.ingested))
	}
	b := sink.ingested[0]
	if b.Country != "GR" || b.City != "Athens" || b.Lat != 38.0 || b.Lon != 23.7 || !b.HasGeo {
		t.Fatalf("beacon geo = %+v", b)
	}
}

type captureGeo struct{ calls int }

func (g *captureGeo) Lookup(string) beacon.Location {
	g.calls++
	return beacon.Location{
		Country: "GR",
		City:    "Example City",
		Lat:     37.9,
		Lon:     23.7,
		HasGeo:  true,
	}
}

func TestCaptureGeolocationAtReceiver(t *testing.T) {
	for _, mode := range []string{"", config.GeolocationOff, config.GeolocationCountry, config.GeolocationCity} {
		t.Run("mode="+mode, func(t *testing.T) {
			cfg := testCfg()
			if mode != "" {
				cfg.Sites[0].Capture = &config.Capture{Geolocation: new(mode)}
			}
			sink, geo := newRecSink(), &captureGeo{}
			handler := newFixture(cfg, sink, geo).Handler()
			req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(faroBody))
			req.RemoteAddr = "198.51.100.20:12345"
			req.Header.Set("Origin", "https://shop.example.com")
			req.Header.Set("User-Agent", browserUA)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, http.StatusAccepted, response.Code)
			require.Len(t, sink.ingested, 1)
			b := sink.ingested[0]
			if mode == config.GeolocationOff {
				assert.Zero(t, geo.calls)
				assert.Empty(t, b.Country)
			} else {
				assert.Equal(t, 1, geo.calls)
				assert.Equal(t, "GR", b.Country)
			}
			if mode == config.GeolocationCity {
				assert.Equal(t, "Example City", b.City)
				assert.Equal(t, float64(37.9), b.Lat)
				assert.Equal(t, float64(23.7), b.Lon)
				assert.True(t, b.HasGeo)
			} else {
				assert.Empty(t, b.City)
				assert.Zero(t, b.Lat)
				assert.Zero(t, b.Lon)
				assert.False(t, b.HasGeo)
			}
		})
	}
}

func TestCaptureCredentialRedactionPreservesProtocolSemantics(t *testing.T) {
	body := `{"meta":{"session":{"id":"session"},"page":{"id":"document-1","url":"https://shop.example.com/"}},"events":[
	 {"name":"faro.performance.navigation","attributes":{"observation_id":"navigation-1","observation_sequence":"2","pageLoadTime":"1000","domContentLoadHandlerTime":"10"}},
	 {"name":"faro.performance.resource","attributes":{"observation_id":"resource-1","observation_sequence":"3","name":"app.js?private=value#fragment","httpHost":"cdn.example.com","duration":"1000","transferSize":"1000","initiatorType":"fetch"}},
	 {"name":"faro.tracing.fetch","attributes":{"url.full":"https://user:password@api.example.com/api?private=value#fragment","http.request.method":"POST","http.response.status_code":"200","duration_ns":"1000"}},
	 {"name":"rage_click","attributes":{"target":"button#pay"}}
	]}`
	for _, token := range []string{"document-1", "navigation-1", "resource-1", "observation_id", "observation_sequence", "unrelated-token", "resource", "duration", "1000", "rage_click", "url.full"} {
		t.Run(token, func(t *testing.T) {
			cfg := testCfg()
			cfg.Sites[0].EventLogs = &config.EventLogs{Destination: config.Destination{AuthToken: token}}
			sink := newRecSink()
			handler := newFixture(cfg, sink, nil).Handler()
			req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(body))
			req.Header.Set("Origin", "https://shop.example.com")
			req.Header.Set("User-Agent", browserUA)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, http.StatusAccepted, response.Code)
			require.Len(t, sink.ingested, 1)
			b := sink.ingested[0]
			assert.Equal(t, "document-1", b.ExperienceID)
			require.NotNil(t, b.Navigation)
			assert.EqualValues(t, 2, b.Navigation.Revision)
			assert.Equal(t, float64(1000), b.Navigation.LoadMS)
			require.Len(t, b.Resources, 1)
			assert.Equal(t, "resource-1", b.Resources[0].ID)
			assert.True(t, b.Resources[0].HasDuration)
			assert.Equal(t, float64(1000), b.Resources[0].DurationMS)
			assert.Equal(t, float64(1000), b.Resources[0].TransferB)
			require.Len(t, b.Events, 3, "disabled frustration must not become a generic event")
			assert.Equal(t, "app.js", b.Events[1].Attrs["name"])
			assert.Equal(t, "https://api.example.com/api", b.Events[2].Attrs["url.full"])
			assert.Equal(t, "1000", b.Events[2].Attrs["duration_ns"])
		})
	}
}

type samplingGeo struct{ calls atomic.Int64 }

func (g *samplingGeo) Lookup(string) beacon.Location {
	g.calls.Add(1)
	return beacon.Location{
		Country: "GR",
		City:    "Athens",
		Lat:     38,
		Lon:     23.7,
		HasGeo:  true,
	}
}

func samplingConfig(t *testing.T, rate float64) *fixtureConfig {
	t.Helper()
	cfg := testCfg()
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"measure_sample_rate":%g}`, rate)), &cfg.Sites[0]))
	return cfg
}

func TestSamplingZeroDiscardsBeforeDecodeAndProcessing(t *testing.T) {
	// Malformed data proves that disabled collection does not run the decoder.
	// Both a stale SDK beacon and a custom producer receive a quiet success.
	for name, body := range map[string]string{"stale page": faroBody, "custom malformed payload": "not JSON"} {
		t.Run(name, func(t *testing.T) {
			sink, geo := newRecSink(), &samplingGeo{}
			_, ts := newTestServer(t, samplingConfig(t, 0), sink, geo)
			resp := post(t, ts, "/rum/shop/collect", body, map[string]string{
				"Origin": "https://shop.example.com", "X-Forwarded-For": "8.8.8.8",
			})
			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
			assert.Equal(t, "https://shop.example.com", resp.Header.Get("Access-Control-Allow-Origin"))
			accepted, rejected := sink.snapshot()
			assert.Zero(t, accepted)
			assert.Empty(t, rejected)
			assert.Zero(t, geo.calls.Load())
		})
	}
}

func TestSamplingZeroPreservesAdmissionPolicies(t *testing.T) {
	for name, tc := range map[string]struct {
		origin  string
		ua      string
		body    string
		chunked bool
		status  int
		reject  string
	}{
		"origin":       {origin: "https://other.example.net", body: faroBody, status: http.StatusForbidden, reject: beacon.RejectOrigin},
		"known size":   {body: strings.Repeat("x", 2049), status: http.StatusRequestEntityTooLarge, reject: beacon.RejectSize},
		"unknown size": {body: strings.Repeat("x", 2049), chunked: true, status: http.StatusRequestEntityTooLarge, reject: beacon.RejectSize},
		"bot":          {ua: "Googlebot", body: faroBody, status: http.StatusNoContent, reject: beacon.RejectBot},
	} {
		t.Run(name, func(t *testing.T) {
			sink, geo := newRecSink(), &samplingGeo{}
			_, ts := newTestServer(t, samplingConfig(t, 0), sink, geo)
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/rum/shop/collect", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Origin", "https://shop.example.com")
			req.Header.Set("User-Agent", browserUA)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.ua != "" {
				req.Header.Set("User-Agent", tc.ua)
			}
			if tc.chunked {
				req.ContentLength = -1
			}
			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, tc.status, resp.StatusCode)
			accepted, rejected := sink.snapshot()
			assert.Zero(t, accepted)
			assert.Equal(t, map[string]int{tc.reject: 1}, rejected)
			assert.Zero(t, geo.calls.Load())
		})
	}
}

func TestSamplingZeroPreservesRateLimit(t *testing.T) {
	cfg := samplingConfig(t, 0)
	cfg.RateLimit.PerIPPerMin = 1
	sink := newRecSink()
	s, ts := newTestServer(t, cfg, sink, nil)
	s.now = func() time.Time { return time.Unix(100, 0) }
	for _, status := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://shop.example.com"})
		assert.Equal(t, status, resp.StatusCode)
	}
	accepted, rejected := sink.snapshot()
	assert.Zero(t, accepted)
	assert.Equal(t, map[string]int{beacon.RejectRate: 1}, rejected)
}

func TestSamplingPositiveRatesDoNotResampleReceivedBeacons(t *testing.T) {
	for name, rate := range map[string]float64{"fraction": 0.000001, "full": 1} {
		t.Run(name, func(t *testing.T) {
			cfg := samplingConfig(t, rate)
			cfg.RateLimit.PerIPPerMin = 1000
			sink, geo := newRecSink(), &samplingGeo{}
			_, ts := newTestServer(t, cfg, sink, geo)
			for i := 0; i < 20; i++ {
				resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{
					"Origin": "https://shop.example.com", "X-Forwarded-For": "8.8.8.8",
				})
				assert.Equal(t, http.StatusAccepted, resp.StatusCode)
			}
			accepted, rejected := sink.snapshot()
			assert.Equal(t, 20, accepted)
			assert.Empty(t, rejected)
			assert.EqualValues(t, 20, geo.calls.Load())
		})
	}
}
