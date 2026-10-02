// SPDX-License-Identifier: GPL-3.0-or-later
package runtimehub_test

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observedRoutes struct {
	hub      *runtimehub.Hub
	acquired chan struct{}
	once     sync.Once
}

func (r *observedRoutes) Acquire(key string) (*ingest.Route, context.Context, func(), bool) {
	route, ctx, release, ok := r.hub.Acquire(key)
	if ok {
		r.once.Do(func() { close(r.acquired) })
	}
	return route, ctx, release, ok
}

// A site must retire even when an admitted client stops sending its body.
func TestRetirementInterruptsNetworkBody(t *testing.T) {
	hub := runtimehub.New()
	aggregator := agg.New(time.Minute)
	route := ingest.NewRoute(config.RumSite{
		Key:            "shop",
		AllowedOrigins: []string{"https://example.org"},
	}, aggregator)
	retire, err := hub.Register("shop", &runtimehub.Site{
		Route:      route,
		Aggregator: aggregator,
		Generation: "first",
	})
	require.NoError(t, err)
	observed := &observedRoutes{
		hub:      hub,
		acquired: make(chan struct{}),
	}
	transport := config.RumCfg{
		MaxBodyBytes: 262144,
		RateLimit: config.RumRateLimit{
			PerIPPerMin:   120,
			PerSitePerSec: 500,
		},
	}
	server := httptest.NewServer(ingest.New(&transport, observed, nil).Handler())
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
	hub := runtimehub.New()
	oldRetire, err := hub.Register("first", &runtimehub.Site{
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
	nextRetire, err := hub.Register("first", &runtimehub.Site{
		Generation: "new",
	})
	require.NoError(t, err)
	defer nextRetire()
	otherRetire, err := hub.Register("second", &runtimehub.Site{
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
	hub := runtimehub.New()
	old := hub.PublishReceiver(runtimehub.Availability{
		Serving: true,
		Listen:  "old",
	})
	next := hub.PublishReceiver(runtimehub.Availability{
		Serving: true,
		Listen:  "next",
	})
	old()
	assert.Equal(t, "next", hub.Availability().Listen)
	assert.True(t, hub.Availability().Serving)
	next()
	assert.False(t, hub.Availability().Serving)
	assert.Equal(t, "next", hub.Availability().Listen)
}
