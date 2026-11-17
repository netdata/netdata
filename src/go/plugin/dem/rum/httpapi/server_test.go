// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

// recSink records outcomes for assertions.
type recSink struct {
	mu       sync.Mutex
	ingested []*beacon.Beacon
	rejects  map[string]int // reason → n
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

func (r *recSink) Reject(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejects[reason]++
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

func (g fixedGeo) Lookup(string) beacon.Location {
	return beacon.Location{
		Country: string(g),
	}
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

const faroBody = `{"meta":{"page":{"id":"document-1","url":"https://shop.example.com/users/42?x=1"},"session":{"id":"s1"},"browser":{"name":"Chrome","version":"120","os":"Linux","mobile":false}},"measurements":[{"type":"web-vitals","values":{"lcp":1234.5},"context":{"id":"vital-1","observation_sequence":"2"},"timestamp":"2030-01-01T00:00:00Z"}],"events":[{"name":"document_activated","attributes":{"observation_id":"document-1","observation_sequence":"1"}}]}`

func newTestServer(
	t *testing.T,
	cfg *fixtureConfig,
	sink Processor,
	geo Locator,
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
