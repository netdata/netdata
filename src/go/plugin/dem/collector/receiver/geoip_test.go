// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const geoIPFixture = "../../../../../crates/netflow-plugin/testdata/mmdb/GeoLite2-City-Test.mmdb"

func replaceGeoIPFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	f, err := os.CreateTemp(filepath.Dir(path), "replacement-")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Rename(f.Name(), path))
}

func TestGeoIPPreflightDoesNotLoadDatabase(t *testing.T) {
	fixture, err := os.ReadFile(geoIPFixture)
	require.NoError(t, err)
	for _, kind := range []string{"missing", "invalid", "valid"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "geo.mmdb")
			switch kind {
			case "invalid":
				replaceGeoIPFile(t, path, []byte("invalid MMDB"))
			case "valid":
				replaceGeoIPFile(t, path, fixture)
			}
			c := New(Dependencies{
				Registry: rumregistry.New(),
			})
			c.GeoIPDB = path
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			status := c.geo.Status()
			assert.Equal(t, "explicit", status.Selection)
			assert.Equal(t, "not_checked", status.Reason)
			assert.Zero(t, status.LastCheckedAt)
			assert.Zero(t, status.LoadedAt)
			assert.Empty(t, status.DatabaseType)
			assert.False(t, c.registry.Availability().Serving)
		})
	}
}

func TestGeoIPRunPublishesAndJoinsWorker(t *testing.T) {
	fixture, err := os.ReadFile(geoIPFixture)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "stock.mmdb")
	replaceGeoIPFile(t, path, fixture)
	hub := rumregistry.New()
	c := New(Dependencies{
		Registry: hub,
		GeoIPPaths: geoip.Paths{
			Stock: path,
		},
	})
	c.Listen = "127.0.0.1:0"
	require.NoError(t, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	done, ready := make(chan struct{}), make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = c.Run(ctx, func() { close(ready) })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			c.Cleanup(context.Background())
		case <-time.After(5 * time.Second):
			t.Error("receiver failed to stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver did not become ready")
	}
	require.Eventually(
		t,
		func() bool { return hub.Availability().GeoIP.State == "loaded" },
		5*time.Second,
		time.Millisecond,
	)
	state := hub.Availability()
	assert.True(t, state.Serving)
	assert.Equal(t, "stock", state.GeoIP.Source)
	assert.Equal(t, "GeoLite2-City", state.GeoIP.DatabaseType)
	assert.NotZero(t, state.GeoIP.LoadedAt)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver failed to join its worker")
	}
	require.NoError(t, runErr)
	state = hub.Availability()
	assert.False(t, state.Serving)
	assert.Equal(t, "receiver_stopped", state.GeoIP.Reason)
	assert.Empty(t, state.GeoIP.Source)
	assert.Zero(t, state.GeoIP.LoadedAt)
	// Cleanup immediately after Run is safe only once background use has ended.
	c.Cleanup(context.Background())
	collectGeoIPStatus(t, c)
	assert.Equal(t, state, hub.Availability())
}

func TestGeoIPBindFailureDoesNotStartWorker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	c := New(Dependencies{
		Registry: rumregistry.New(),
		GeoIPPaths: geoip.Paths{
			Stock: geoIPFixture,
		},
	})
	c.Listen = ln.Addr().String()
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	require.NoError(t, c.Init(context.Background()))
	ready := false
	require.Error(t, c.Run(context.Background(), func() { ready = true }))
	assert.False(t, ready)
	assert.False(t, c.registry.Availability().Serving)
	assert.Equal(t, "not_checked", c.geo.Status().Reason)
	assert.Zero(t, c.geo.Status().LastCheckedAt)
}

func TestGeoIPWorkerAdoptsLateAndReplacedData(t *testing.T) {
	fixture, err := os.ReadFile(geoIPFixture)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "late", "stock.mmdb")
	hub := rumregistry.New()
	c := New(Dependencies{
		Registry: hub,
		GeoIPPaths: geoip.Paths{
			Stock: path,
		},
	})
	require.NoError(t, c.Init(context.Background()))
	c.publication = hub.PublishReceiver(rumregistry.Availability{
		Serving: true,
		GeoIP:   c.geo.Status(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	ticks, done := make(chan time.Time), make(chan struct{})
	go func() { defer close(done); c.runGeoIP(ctx, ticks) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			c.publication.Close()
			c.Cleanup(context.Background())
		case <-time.After(5 * time.Second):
			t.Error("GeoIP worker failed to stop")
		}
	})
	require.Eventually(
		t,
		func() bool { return hub.Availability().GeoIP.Reason == "missing" },
		5*time.Second,
		time.Millisecond,
	)
	tick := func() {
		t.Helper()
		select {
		case ticks <- time.Now():
		case <-time.After(5 * time.Second):
			t.Fatal("GeoIP worker did not accept refresh tick")
		}
	}
	replaceGeoIPFile(t, path, fixture)
	tick()
	require.Eventually(
		t,
		func() bool { return hub.Availability().GeoIP.State == "loaded" },
		5*time.Second,
		time.Millisecond,
	)
	loaded := hub.Availability().GeoIP
	replaceGeoIPFile(t, path, []byte("invalid atomic replacement"))
	tick()
	require.Eventually(
		t,
		func() bool { return hub.Availability().GeoIP.State == "using_previous" },
		5*time.Second,
		time.Millisecond,
	)
	assert.Equal(t, loaded.LoadedAt, hub.Availability().GeoIP.LoadedAt)
	replaceGeoIPFile(t, path, fixture)
	tick()
	require.Eventually(t, func() bool {
		status := hub.Availability().GeoIP
		return status.State == "loaded" && status.LoadedAt > loaded.LoadedAt
	}, 5*time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GeoIP worker did not honor cancellation")
	}
	stopped := hub.Availability().GeoIP
	assert.Equal(t, stopped, c.geo.Status())
}

func TestGeoIPCollectPublishesRecordErrors(t *testing.T) {
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "GeoLite2-Country",
		IPVersion:               4,
		IncludeReservedNetworks: true,
	})
	require.NoError(t, err)
	_, network, err := net.ParseCIDR("192.0.2.0/24")
	require.NoError(t, err)
	require.NoError(t, tree.Insert(network, mmdbtype.Map{
		"country": mmdbtype.Map{
			"iso_code": mmdbtype.Uint32(42),
		},
	}))
	var data bytes.Buffer
	_, err = tree.WriteTo(&data)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "malformed-record.mmdb")
	replaceGeoIPFile(t, path, data.Bytes())
	hub := rumregistry.New()
	c := New(Dependencies{
		Registry: hub,
		GeoIPPaths: geoip.Paths{
			Stock: path,
		},
	})
	require.NoError(t, c.Init(context.Background()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	require.NoError(t, c.geo.Refresh())
	c.publication = hub.PublishReceiver(rumregistry.Availability{
		Serving: true,
		GeoIP:   c.geo.Status(),
	})
	t.Cleanup(c.publication.Close)
	country, _, _, _, _ := c.geo.Lookup("192.0.2.1")
	assert.Empty(t, country)
	assert.Zero(t, hub.Availability().GeoIP.LookupErrors)
	collectGeoIPStatus(t, c)
	assert.Equal(t, uint64(1), hub.Availability().GeoIP.LookupErrors)
	assert.Equal(t, "loaded", hub.Availability().GeoIP.State)
}

func collectGeoIPStatus(t *testing.T, c *Collector) {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
}
