// SPDX-License-Identifier: GPL-3.0-or-later
package registry_test

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type measurementProcessor struct{ *aggregate.Aggregator }

func (p measurementProcessor) Ingest(b *beacon.Beacon) { p.Aggregator.Ingest(b) }

type observedRoutes struct {
	hub      *rumregistry.Registry
	acquired chan struct{}
	once     sync.Once
}

func (r *observedRoutes) Acquire(key string) (*httpapi.Route, context.Context, func(), bool) {
	route, ctx, release, ok := r.hub.Acquire(key)
	if ok {
		r.once.Do(func() { close(r.acquired) })
	}
	return route, ctx, release, ok
}

// A site must retire even when an admitted client stops sending its body.
func TestRetirementInterruptsNetworkBody(t *testing.T) {
	hub := rumregistry.New()
	aggregator := aggregate.New(time.Minute, aggregate.SiteCfg{
		Name: "shop",
	})
	cfg := config.Site{
		Name:           "shop",
		AllowedOrigins: []string{"https://example.org"},
	}
	state := diagnostics.New()
	route := httpapi.NewRoute(cfg, measurementProcessor{aggregator}, state)
	retire, err := hub.Register("shop", &rumregistry.Site{
		Route:       route,
		Diagnostics: state,
		Aggregator:  aggregator,
		Generation:  "first",
	})
	require.NoError(t, err)
	observed := &observedRoutes{
		hub:      hub,
		acquired: make(chan struct{}),
	}
	transport := config.Receiver{
		MaxBodyBytes: 262144,
		RateLimit: config.RateLimit{
			PerIPPerMin:   120,
			PerSitePerSec: 500,
		},
	}
	server := httptest.NewServer(httpapi.New(&transport, observed, nil).Handler())
	defer server.Close()
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprint(
		conn,
		"POST /rum/shop/collect HTTP/1.1\r\nHost: localhost\r\nOrigin: https://example.org\r\nContent-Length: 1000\r\n\r\n{",
	)
	require.NoError(t, err)
	select {
	case <-observed.acquired:
	case <-time.After(time.Second):
		t.Fatal("request did not acquire its route")
	}
	done := make(chan struct{})
	go func() { retire(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		_ = conn.Close()
		<-done
		t.Fatal("retirement blocked on an unfinished client body")
	}
	_, _, _, ok := hub.AcquireSite("shop")
	assert.False(t, ok)
}
func TestRetirementIsIndependentAndGenerationFenced(t *testing.T) {
	hub := rumregistry.New()
	oldRetire, err := hub.Register("first", &rumregistry.Site{
		Generation: "old",
	})
	require.NoError(t, err)
	_, ctx, release, ok := hub.AcquireSite("first")
	require.True(t, ok)
	done := make(chan struct{})
	go func() { oldRetire(); close(done) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lease was not revoked")
	}
	nextRetire, err := hub.Register("first", &rumregistry.Site{
		Generation: "new",
	})
	require.NoError(t, err)
	defer nextRetire()
	otherRetire, err := hub.Register("second", &rumregistry.Site{
		Generation: "other",
	})
	require.NoError(t, err)
	defer otherRetire()
	for _, key := range []string{"first", "second"} {
		data, _, end, ok := hub.AcquireSite(key)
		require.True(t, ok)
		assert.NotEqual(t, "old", data.Generation)
		end()
	}
	select {
	case <-done:
		t.Fatal("retirement returned before its reader released")
	default:
	}
	release()
	<-done
	oldRetire()
	data, _, end, ok := hub.AcquireSite("first")
	require.True(t, ok)
	assert.Equal(t, "new", data.Generation)
	end()
}
func TestReceiverAvailabilityIsGenerationFenced(t *testing.T) {
	hub := rumregistry.New()
	old := hub.PublishReceiver(rumregistry.Availability{
		Serving:   true,
		PublicURL: "https://old.example",
	})
	next := hub.PublishReceiver(rumregistry.Availability{
		Serving:   true,
		PublicURL: "https://next.example",
	})
	next.SetGeoIP(
		geoip.Status{
			Selection:    "explicit",
			State:        "loaded",
			Source:       "explicit",
			DatabaseType: "GeoLite2-Country",
			LookupErrors: 3,
		},
	)
	old.SetGeoIP(geoip.Status{
		State:  "using_previous",
		Source: "cache",
	})
	old.Close()
	assert.Equal(t, "explicit", hub.Availability().GeoIP.Source)
	assert.Equal(t, uint64(3), hub.Availability().GeoIP.LookupErrors)
	assert.Equal(t, "https://next.example", hub.Availability().PublicURL)
	assert.True(t, hub.Availability().Serving)
	next.Close()
	next.SetGeoIP(geoip.Status{
		State: "loaded",
	})
	assert.Equal(t, "unavailable", hub.Availability().GeoIP.State)
	assert.Equal(t, "receiver_stopped", hub.Availability().GeoIP.Reason)
	assert.Empty(t, hub.Availability().GeoIP.DatabaseType)
	assert.False(t, hub.Availability().Serving)
	assert.Equal(t, "https://next.example", hub.Availability().PublicURL)
}
