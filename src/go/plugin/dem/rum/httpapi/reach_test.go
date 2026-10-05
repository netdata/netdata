// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"

	"github.com/stretchr/testify/require"
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
	key := testCfg().Sites[0].Name
	getVia(
		t,
		ts,
		"/rum/"+key+".js",
		map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "rum.example.com"},
	)
	if got := s.route(key).diagnostics.ObservedBase(); got != "https://rum.example.com" {
		t.Fatalf("observed base = %q", got)
	}
}

// A direct request can claim any Host; it must never become the snippet.
func TestBootstrapFromUntrustedPeerRecordsNothing(t *testing.T) {
	cfg := testCfg()
	cfg.TrustedProxies = nil
	s, ts := newTestServer(t, cfg, newRecSink(), nil)
	key := cfg.Sites[0].Name
	getVia(t, ts, "/rum/"+key+".js", map[string]string{"X-Forwarded-Host": "evil.example"})
	if got := s.route(key).diagnostics.ObservedBase(); got != "" {
		t.Fatalf("observed base from an untrusted peer = %q", got)
	}
}

func TestProbeReachesItsOwnBootstrap(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	key := testCfg().Sites[0].Name
	r := s.route(key).diagnostics.Probe(context.Background(), ts.Client(), ts.URL)
	if r.State != diagnostics.ReachOK || r.Error != "" {
		t.Fatalf("probe = %+v", r)
	}
	if got, ok := s.route(key).diagnostics.Reachability(); !ok || got.State != diagnostics.ReachOK {
		t.Fatalf("recorded = %+v %v", got, ok)
	}
}

func TestProbeReportsWhatBreaks(t *testing.T) {
	s, _ := newTestServer(t, testCfg(), newRecSink(), nil)
	key := testCfg().Sites[0].Name

	notOurs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>some other server</html>"))
	}))
	defer notOurs.Close()
	if r := s.route(key).diagnostics.Probe(context.Background(), notOurs.Client(), notOurs.URL); r.State != diagnostics.ReachFailed ||
		r.Error == "" {
		t.Fatalf("a server that is not this collector must fail: %+v", r)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	if r := s.route(key).diagnostics.Probe(context.Background(), http.DefaultClient, gone.URL); r.State != diagnostics.ReachFailed ||
		r.Error == "" {
		t.Fatalf("an unreachable address must fail: %+v", r)
	}
}

// Snippet host: configured public_url first; a learned address only once
// a probe confirmed it serves this collector; otherwise the listener.
func TestPublicBasePrefersConfiguredThenConfirmedObserved(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	site := testCfg().Sites[0]
	const fallback = "http://127.0.0.1:19938"

	if got := s.route(site.Name).diagnostics.PublicBase(fallback); got != fallback {
		t.Fatalf("nothing known: %q", got)
	}
	getVia(t, ts, "/rum/"+site.Name+".js", nil)
	if got := s.route(site.Name).diagnostics.PublicBase(fallback); got != fallback {
		t.Fatalf("an unconfirmed observed address must not be used: %q", got)
	}
	s.route(
		site.Name,
	).diagnostics.Probe(
		context.Background(),
		ts.Client(),
		s.route(site.Name).diagnostics.ObservedBase(),
	)
	if got := s.route(site.Name).diagnostics.PublicBase(fallback); got != ts.URL {
		t.Fatalf("confirmed observed address: got %q want %q", got, ts.URL)
	}
	site.PublicURL = "https://rum.configured.example/"
	configured := diagnostics.New(site)
	if got := configured.PublicBase(fallback); got != "https://rum.configured.example" {
		t.Fatalf("configured public_url must win: %q", got)
	}
}

func TestRunReachabilityProbesItsSite(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	site := testCfg().Sites[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.route(site.Name).diagnostics.RunReachability(ctx, time.Hour, func() string { return ts.URL }, ts.Client())
	}()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if r, ok := s.route(site.Name).diagnostics.Reachability(); ok && r.State == diagnostics.ReachOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("first probe did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestObservedAddressRequiresItsOwnSuccessfulProbe(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	route := s.route("shop")
	const fallback = "http://127.0.0.1:19938"
	_, checked := route.diagnostics.Reachability()
	require.False(t, checked)
	_, checked = route.diagnostics.Snippet()
	require.False(t, checked)
	_, rejected := route.diagnostics.LastRejectedOrigin()
	require.False(t, rejected)
	getVia(t, ts, "/rum/shop.js", nil)
	require.Equal(t, diagnostics.ReachOK, route.diagnostics.Probe(context.Background(), ts.Client(), ts.URL).State)
	require.Equal(t, ts.URL, route.diagnostics.PublicBase(fallback))
	getVia(
		t,
		ts,
		"/rum/shop.js",
		map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "new.example.org"},
	)
	require.Equal(t, "https://new.example.org", route.diagnostics.ObservedBase())
	require.Equal(
		t,
		fallback,
		route.diagnostics.PublicBase(fallback),
		"an old successful probe cannot confirm a different address",
	)
}
