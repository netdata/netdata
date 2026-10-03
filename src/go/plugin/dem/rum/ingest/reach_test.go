// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
)

func getVia(t *testing.T, ts *httptest.Server, path string, hdr map[string]string) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

// The address browsers use is learned from requests that came through a
// trusted proxy (tunnel, reverse proxy), which is what sets the Host.
func TestBootstrapThroughTrustedProxyRecordsPublicBase(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	key := testCfg().Sites[0].Key
	getVia(
		t,
		ts,
		"/rum/"+key+".js",
		map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "rum.example.com"},
	)
	if got := s.ObservedBase(key); got != "https://rum.example.com" {
		t.Fatalf("observed base = %q", got)
	}
}

// A direct request can claim any Host; it must never become the snippet.
func TestBootstrapFromUntrustedPeerRecordsNothing(t *testing.T) {
	cfg := testCfg()
	cfg.TrustedProxies = nil
	s, ts := newTestServer(t, cfg, newRecSink(), nil)
	key := cfg.Sites[0].Key
	getVia(t, ts, "/rum/"+key+".js", map[string]string{"X-Forwarded-Host": "evil.example"})
	if got := s.ObservedBase(key); got != "" {
		t.Fatalf("observed base from an untrusted peer = %q", got)
	}
}

func TestProbeReachesItsOwnBootstrap(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	key := testCfg().Sites[0].Key
	r := s.Probe(context.Background(), ts.Client(), key, ts.URL)
	if r.State != ReachOK || r.Error != "" {
		t.Fatalf("probe = %+v", r)
	}
	if got, ok := s.Reachability(key); !ok || got.State != ReachOK {
		t.Fatalf("recorded = %+v %v", got, ok)
	}
}

func TestProbeReportsWhatBreaks(t *testing.T) {
	s, _ := newTestServer(t, testCfg(), newRecSink(), nil)
	key := testCfg().Sites[0].Key

	notOurs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>some other server</html>"))
	}))
	defer notOurs.Close()
	if r := s.Probe(context.Background(), notOurs.Client(), key, notOurs.URL); r.State != ReachFailed || r.Error == "" {
		t.Fatalf("a server that is not this collector must fail: %+v", r)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	if r := s.Probe(context.Background(), http.DefaultClient, key, gone.URL); r.State != ReachFailed || r.Error == "" {
		t.Fatalf("an unreachable address must fail: %+v", r)
	}
}

// Snippet host: configured public_url first; a learned address only once
// a probe confirmed it serves this collector; otherwise the listener.
func TestPublicBasePrefersConfiguredThenConfirmedObserved(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	site := testCfg().Sites[0]
	const fallback = "http://127.0.0.1:19938"

	if got := s.PublicBase(site, fallback); got != fallback {
		t.Fatalf("nothing known: %q", got)
	}
	getVia(t, ts, "/rum/"+site.Key+".js", nil)
	if got := s.PublicBase(site, fallback); got != fallback {
		t.Fatalf("an unconfirmed observed address must not be used: %q", got)
	}
	s.Probe(context.Background(), ts.Client(), site.Key, s.ObservedBase(site.Key))
	if got := s.PublicBase(site, fallback); got != ts.URL {
		t.Fatalf("confirmed observed address: got %q want %q", got, ts.URL)
	}
	site.PublicURL = "https://rum.configured.example/"
	if got := s.PublicBase(site, fallback); got != "https://rum.configured.example" {
		t.Fatalf("configured public_url must win: %q", got)
	}
}

func TestRunReachabilityProbesEverySite(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	site := testCfg().Sites[0]
	site.PublicURL = ts.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.RunReachability(ctx, time.Hour, func() []config.RumSite { return []config.RumSite{site} }, ts.Client())
	deadline := time.Now().Add(2 * time.Second)
	for {
		if r, ok := s.Reachability(site.Key); ok && r.State == ReachOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("first probe did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Measure sampling happens in the browser: the snippet tells Faro to
// sample sessions, and leaves Faro's default alone at 100%.
func TestBootstrapCarriesMeasureSampling(t *testing.T) {
	js := bootstrapJS("shop", "https://rum.example", &site{
		measureRate: 0.25,
	})
	if !strings.Contains(js, `"sampling":0.25`) ||
		!strings.Contains(js, "cfg.sessionTracking = { samplingRate: opt.sampling }") {
		t.Fatalf("25%% measure sampling missing:\n%s", js)
	}
	if js := bootstrapJS("shop", "https://rum.example", &site{
		measureRate: 1,
	}); !strings.Contains(js, `"sampling":1`) {
		t.Fatalf("100%% sampling not passed:\n%s", js)
	}
}
