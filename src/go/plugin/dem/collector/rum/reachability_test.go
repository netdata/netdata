// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/require"
)

func TestProductionProbeConfirmsTrustedProxyPublicBase(t *testing.T) {
	hub := runtimehub.New()
	db, err := store.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	server := httptest.NewServer(
		ingest.New(&config.Receiver{
			TrustedProxies: []string{"127.0.0.1/32"},
			MaxBodyBytes:   262144,
			RateLimit: config.RateLimit{
				PerIPPerMin:   120,
				PerSitePerSec: 500,
			},
		}, hub, nil).
			Handler(),
	)
	t.Cleanup(server.Close)
	backend, err := url.Parse(server.URL)
	require.NoError(t, err)
	revoke := hub.PublishReceiver(runtimehub.Availability{
		Serving: true,
		Listen:  backend.Host,
	})
	t.Cleanup(revoke)
	site := New(Dependencies{
		Hub:     hub,
		History: db,
	})
	site.Name = "shop"
	site.AllowedOrigins = []string{server.URL}
	site.OTLP.Enabled = "no"
	require.NoError(t, site.Init(context.Background()))
	route := ingest.NewRoute(site.Site, site.aggregator)
	retire, err := hub.Register(
		site.Name,
		&runtimehub.Site{
			Route:      route,
			Aggregator: site.aggregator,
			Generation: "test",
		},
	)
	require.NoError(t, err)
	t.Cleanup(retire)

	proxyHandler := httputil.NewSingleHostReverseProxy(backend)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Forwarded-Host", r.Host)
		r.Header.Set("X-Forwarded-Proto", "http")
		proxyHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	resp, err := proxy.Client().Get(proxy.URL + "/rum/shop.js")
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, proxy.URL, route.ObservedBase())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		route.RunReachability(ctx, 10*time.Millisecond, site.probeBase, proxy.Client())
	}()
	t.Cleanup(func() { cancel(); <-done })
	require.Eventually(t, func() bool {
		reach, ok := route.Reachability()
		return ok && reach.State == ingest.ReachOK && reach.Base == proxy.URL
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, proxy.URL, route.PublicBase(server.URL))
}
