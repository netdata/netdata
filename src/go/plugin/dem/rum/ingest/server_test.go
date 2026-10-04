// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

// recSink records outcomes for assertions.
type recSink struct {
	mu       sync.Mutex
	ingested []*beacon.Beacon
	rejects  map[string]int // site+"/"+reason → n
}

func newRecSink() *recSink {
	return &recSink{
		rejects: map[string]int{},
	}
}

func (r *recSink) Ingest(b *beacon.Beacon) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ingested = append(r.ingested, b)
}

func (r *recSink) Reject(site, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejects[site+"/"+reason]++
}

func (r *recSink) snapshot() (int, map[string]int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.rejects))
	for k, v := range r.rejects {
		out[k] = v
	}
	return len(r.ingested), out
}

type fixedGeo string

func (g fixedGeo) Lookup(string) (string, string, float64, float64, bool) {
	return string(g), "", 0, 0, false
}

func testCfg() *fixtureConfig {
	return &fixtureConfig{
		Receiver: config.Receiver{
			Listen:         "127.0.0.1:0",
			TrustedProxies: []string{"127.0.0.1/32", "::1/128", "10.0.0.0/8"},
			MaxBodyBytes:   2048,
			RateLimit: config.RateLimit{
				PerIPPerMin:   5,
				PerSitePerSec: 100,
			},
		},
		Sites: []config.Site{
			{
				Name:           "shop",
				DisplayName:    "Shop",
				AllowedOrigins: []string{"https://shop.example.com", "https://*.example.org", "http://127.0.0.1:19938"},
				PageGroups:     20,
				Countries:      20,
			},
		},
	}
}

// browserUA is a real browser's; Go's default client UA is a bot.
const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

const faroBody = `{"meta":{"page":{"url":"https://shop.example.com/users/42?x=1"},"session":{"id":"s1"},"browser":{"name":"Chrome","version":"120","os":"Linux","mobile":false}},"measurements":[{"type":"web-vitals","values":{"lcp":1234.5},"timestamp":"2030-01-01T00:00:00Z"}]}`

func newTestServer(
	t *testing.T,
	cfg *fixtureConfig,
	sink beacon.Sink,
	geo CountryResolver,
) (*fixtureServer, *httptest.Server) {
	t.Helper()
	s := newFixture(cfg, sink, geo)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func post(t *testing.T, ts *httptest.Server, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
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
			}, wantStatus: 403, wantReject: "shop/rejected_origin",
		},
		"default port normalized": {
			hdr: map[string]string{
				"Origin": "https://shop.example.com:443",
			}, wantStatus: 202, wantACAO: "https://shop.example.com:443",
		},
		"scheme must match": {
			hdr: map[string]string{
				"Origin": "http://shop.example.com",
			}, wantStatus: 403, wantReject: "shop/rejected_origin",
		},
		"wrong origin": {
			hdr: map[string]string{
				"Origin": "https://evil.example.net",
			}, wantStatus: 403, wantReject: "shop/rejected_origin",
		},
		"missing origin and referer": {
			hdr: map[string]string{}, wantStatus: 403, wantReject: "shop/rejected_origin",
		},
		"null origin": {
			hdr: map[string]string{"Origin": "null"}, wantStatus: 403, wantReject: "shop/rejected_origin",
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

func TestUnknownAndDisabledSites(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(t, testCfg(), sink, nil)
	tests := map[string]struct {
		method, path string
	}{
		"unknown key collect":   {method: http.MethodPost, path: "/rum/nope/collect"},
		"disabled site collect": {method: http.MethodPost, path: "/rum/off/collect"},
		"unknown key bootstrap": {method: http.MethodGet, path: "/rum/nope.js"},
		"disabled bootstrap":    {method: http.MethodGet, path: "/rum/off.js"},
		"root":                  {method: http.MethodGet, path: "/"},
		"other path":            {method: http.MethodGet, path: "/api/rum/aggregate"},
		"preflight unknown":     {method: http.MethodOptions, path: "/rum/nope/collect"},
		"demo unknown key":      {method: http.MethodGet, path: "/rum/demo?key=nope"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(faroBody))
			req.Header.Set("Origin", "https://off.example.com")
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status %d want 404", resp.StatusCode)
			}
		})
	}
	n, rejects := sink.snapshot()
	if n != 0 || len(rejects) != 0 {
		t.Fatalf("unknown/disabled sites must not be counted: %d %v", n, rejects)
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
			wantReject: "shop/rejected_size",
		},
		"exactly at cap": {
			body:       `{"meta":{},"pad":"` + strings.Repeat("x", 2048-len(`{"meta":{},"pad":""}`)) + `"}`,
			wantStatus: 202,
		},
		"one over cap": {
			body:       `{"meta":{},"pad":"` + strings.Repeat("x", 2049-len(`{"meta":{},"pad":""}`)) + `"}`,
			wantStatus: 413,
			wantReject: "shop/rejected_size",
		},
		"bad json":        {body: `{"meta":`, wantStatus: 400, wantReject: "shop/invalid"},
		"not json at all": {body: `hello`, wantStatus: 400, wantReject: "shop/invalid"},
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
	if rejects["shop/rejected_size"] != 1 {
		t.Fatalf("rejects %v", rejects)
	}
}

func TestPerIPRateLimit(t *testing.T) {
	sink := newRecSink()
	s, ts := newTestServer(t, testCfg(), sink, nil)
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	origin := map[string]string{"Origin": "https://shop.example.com"}

	for i := 0; i < 5; i++ {
		if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 202 {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
	}
	resp := post(t, ts, "/rum/shop/collect", faroBody, origin)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("6th request: status %d retry-after %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	n, rejects := sink.snapshot()
	if n != 5 || rejects["shop/rejected_rate"] != 1 {
		t.Fatalf("ingested %d rejects %v", n, rejects)
	}

	// 5/min refills one token every 12s.
	now = now.Add(12 * time.Second)
	if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 202 {
		t.Fatalf("after refill: status %d", resp.StatusCode)
	}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 429 {
		t.Fatalf("second after refill: status %d", resp.StatusCode)
	}
}

func TestPerSiteRateLimit(t *testing.T) {
	cfg := testCfg()
	cfg.RateLimit = config.RateLimit{
		PerIPPerMin:   100000,
		PerSitePerSec: 3,
	}
	sink := newRecSink()
	s, ts := newTestServer(t, cfg, sink, nil)
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	// Distinct client IPs via trusted proxy so the per-IP bucket is not the limiter.
	for i := 0; i < 3; i++ {
		hdr := map[string]string{
			"Origin":          "https://shop.example.com",
			"X-Forwarded-For": "203.0.113." + string(rune('1'+i)),
		}
		if resp := post(t, ts, "/rum/shop/collect", faroBody, hdr); resp.StatusCode != 202 {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
	}
	hdr := map[string]string{"Origin": "https://shop.example.com", "X-Forwarded-For": "203.0.113.9"}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, hdr); resp.StatusCode != 429 {
		t.Fatalf("4th request: status %d want 429", resp.StatusCode)
	}
	_, rejects := sink.snapshot()
	if rejects["shop/rejected_rate"] != 1 {
		t.Fatalf("rejects %v", rejects)
	}
}

func TestClientIPTrustedProxies(t *testing.T) {
	snap := compile(&testCfg().Receiver)
	tests := map[string]struct {
		remote string
		hdr    map[string]string
		want   string
	}{
		"direct client, headers ignored": {
			remote: "203.0.113.5:4242", hdr: map[string]string{"X-Forwarded-For": "198.51.100.1"}, want: "203.0.113.5",
		},
		"trusted proxy, single xff": {
			remote: "127.0.0.1:4242", hdr: map[string]string{"X-Forwarded-For": "198.51.100.1"}, want: "198.51.100.1",
		},
		"trusted proxy, right-most non-trusted wins": {
			remote: "127.0.0.1:4242", hdr: map[string]string{"X-Forwarded-For": "1.1.1.1, 198.51.100.7, 10.1.2.3"}, want: "198.51.100.7",
		},
		"spoofed left entry does not win": {
			remote: "10.0.0.2:1", hdr: map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.8"}, want: "198.51.100.8",
		},
		"all hops trusted falls back to remote": {
			remote: "127.0.0.1:1", hdr: map[string]string{"X-Forwarded-For": "10.9.9.9"}, want: "127.0.0.1",
		},
		"malformed xff falls back to remote": {
			remote: "127.0.0.1:1", hdr: map[string]string{"X-Forwarded-For": "not-an-ip"}, want: "127.0.0.1",
		},
		"x-real-ip from trusted proxy": {
			remote: "127.0.0.1:1", hdr: map[string]string{"X-Real-IP": "198.51.100.9"}, want: "198.51.100.9",
		},
		"x-real-ip from untrusted ignored": {
			remote: "203.0.113.5:1", hdr: map[string]string{"X-Real-IP": "198.51.100.9"}, want: "203.0.113.5",
		},
		"ipv6 remote": {
			remote: "[2001:db8::1]:443", hdr: nil, want: "2001:db8::1",
		},
		"ipv6 loopback trusted": {
			remote: "[::1]:443", hdr: map[string]string{"X-Forwarded-For": "2001:db8::9"}, want: "2001:db8::9",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.hdr {
				r.Header.Set(k, v)
			}
			if got := snap.clientIP(r).String(); got != tc.want {
				t.Fatalf("clientIP = %s want %s", got, tc.want)
			}
		})
	}
}

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

// fullGeo is a CountryResolver stub carrying city/lat/lon too, for asserting the server → parseFaro wiring end to end
// (faro_test.go covers parseFaro's own decoding in isolation).
type fullGeo struct {
	country, city string
	lat, lon      float64
	hasGeo        bool
}

func (g fullGeo) Lookup(string) (string, string, float64, float64, bool) {
	return g.country, g.city, g.lat, g.lon, g.hasGeo
}

func TestGeoCityLatLonAppliedFromResolver(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(
		t,
		testCfg(),
		sink,
		fullGeo{
			country: "GR",
			city:    "Athens",
			lat:     38.0,
			lon:     23.7,
			hasGeo:  true,
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
		Path:           "/users/42",
		PageGroup:      "/users/:id",
		Browser:        "Chrome",
		BrowserVersion: "120",
		OS:             "Linux",
		Device:         "desktop",
		Vitals:         []beacon.Vital{{Name: "LCP", Value: 1234.5, Time: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}
	if got.Site != want.Site || !got.Received.Equal(want.Received) || got.SessionID != want.SessionID ||
		got.Path != want.Path || got.PageGroup != want.PageGroup || got.Browser != want.Browser ||
		got.BrowserVersion != want.BrowserVersion || got.OS != want.OS || got.Device != want.Device ||
		got.Country != "" || len(got.Vitals) != 1 || got.Vitals[0].Name != "LCP" || got.Vitals[0].Value != 1234.5 ||
		!got.Vitals[0].Time.Equal(want.Vitals[0].Time) || got.Errors != nil || got.Events != nil || got.Logs != nil {
		t.Fatalf("beacon mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestBootstrapAndDemo(t *testing.T) {
	cfg := testCfg()
	cfg.Sites[0].Name = "demo"
	sink := newRecSink()
	_, ts := newTestServer(t, cfg, sink, nil)

	resp, err := ts.Client().Get(ts.URL + "/rum/demo.js")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("bootstrap status %d type %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	js := string(body)
	for _, want := range []string{"load('faro-web-sdk'", `'@` + FaroVersion + `/dist/bundle/'`, `})("demo", "` + ts.URL + `", {`, "/rum/' + k + '/collect"} {
		if !strings.Contains(js, want) {
			t.Fatalf("bootstrap missing %q in:\n%s", want, js)
		}
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("bootstrap is a public asset and must not carry ACAO")
	}

	resp, err = ts.Client().Get(ts.URL + "/rum/demo")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `src="/rum/demo.js"`) {
		t.Fatalf("demo status %d body %s", resp.StatusCode, body)
	}
}

func TestBootstrapReadsDataVersionEnv(t *testing.T) {
	js := bootstrapJS("demo", "http://127.0.0.1:19940", &site{
		measureRate: 1,
	})
	for _, want := range []string{
		"document.currentScript",
		"getAttribute('data-version')",
		"getAttribute('data-env')",
		"app.version = ver",
		"app.environment = env",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("bootstrap missing %q in:\n%s", want, js)
		}
	}
}

func TestBootstrapHostQuoting(t *testing.T) {
	js := bootstrapJS("demo", `http://evil"); alert(1); ("`, &site{
		measureRate: 1,
	})
	if strings.Contains(js, `alert(1); (`) && !strings.Contains(js, `\"`) {
		t.Fatalf("host not quoted: %s", js)
	}
	if !strings.Contains(js, `})("demo", "http://evil\"); alert(1); (\"", {`) {
		t.Fatalf("unexpected quoting: %s", js)
	}
}

func TestUpdateSwapsSnapshot(t *testing.T) {
	sink := newRecSink()
	s, ts := newTestServer(t, testCfg(), sink, nil)
	origin := map[string]string{"Origin": "https://shop.example.com"}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 202 {
		t.Fatalf("before update: %d", resp.StatusCode)
	}
	cfg := testCfg()
	cfg.Sites[0].AllowedOrigins = []string{"https://other.example.com"}
	s.Update(cfg)
	if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 403 {
		t.Fatalf("after update old origin: %d want 403", resp.StatusCode)
	}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://other.example.com"}); resp.StatusCode != 202 {
		t.Fatalf("after update new origin: %d want 202", resp.StatusCode)
	}
}

func TestLimiterSweepBoundsMemory(t *testing.T) {
	l := newLimiter(1, 2, 10)
	now := time.Unix(0, 0)
	for i := 0; i < 10; i++ {
		l.allow("ip"+string(rune('a'+i)), now)
	}
	// Idle long enough for every bucket to refill: adding the 11th key sweeps them.
	now = now.Add(5 * time.Second)
	l.allow("fresh", now)
	if len(l.buckets) != 1 {
		t.Fatalf("buckets after sweep = %d want 1", len(l.buckets))
	}
}

// A full table of still-active buckets cannot be swept; scanning it again
// for every new client serialized ingest (one full scan per beacon from a
// new IP). Sweeps are amortized: at most one per second.
func TestLimiterSweepIsAmortizedWhenNothingIsIdle(t *testing.T) {
	l := newLimiter(2, 120, 100)
	now := time.Unix(0, 0)
	for i := 0; i < 100; i++ {
		l.allow(fmt.Sprintf("busy-%d", i), now)
	}
	for i := 0; i < 5000; i++ {
		l.allow(fmt.Sprintf("new-%d", i), now.Add(time.Duration(i)*time.Microsecond))
	}
	if l.sweeps > 1 {
		t.Fatalf("%d sweeps for 5000 new clients within one second, want at most 1", l.sweeps)
	}
	now = now.Add(2 * time.Second)
	l.allow("later", now)
	if l.sweeps != 2 {
		t.Fatalf("a new client a second later must sweep again: sweeps = %d", l.sweeps)
	}
}

// The snippet is revalidated, never cached for a fixed time: an
// unchanged snippet answers 304, a settings change a new body.
func TestBootstrapRevalidates(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	get := func(etag string) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+"/rum/shop.js", nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	first := get("")
	etag := first.Header.Get("ETag")
	if first.StatusCode != 200 || first.Header.Get("Cache-Control") != "private, no-cache" || etag == "" {
		t.Fatalf("status %d cache %q etag %q", first.StatusCode, first.Header.Get("Cache-Control"), etag)
	}
	if resp := get(etag); resp.StatusCode != http.StatusNotModified {
		t.Fatalf("unchanged snippet: %d, want 304", resp.StatusCode)
	}
	cfg := testCfg()
	cfg.Sites[0].MeasureSampleRate = 0.5
	s.Update(cfg)
	if resp := get(etag); resp.StatusCode != 200 || resp.Header.Get("ETag") == etag {
		t.Fatalf("changed settings must send a new snippet: %d", resp.StatusCode)
	}
}
