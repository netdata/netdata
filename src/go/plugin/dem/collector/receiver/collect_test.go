// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every served request lands in exactly one status-class counter.
func TestHTTPRequestsAreCountedByStatusClass(t *testing.T) {
	hub := rumregistry.New()
	c := New(Dependencies{
		Registry: hub,
	})
	c.Listen = "127.0.0.1:0"
	require.NoError(t, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	done, ready := make(chan error, 1), make(chan struct{})
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("receiver failed to stop")
		}
		c.Cleanup(context.Background())
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver did not become ready")
	}
	base := "http://" + hub.Availability().Listen
	for path, status := range map[string]int{
		"/rum/ping":       http.StatusNoContent,
		"/rum/missing.js": http.StatusNotFound,
		"/elsewhere":      http.StatusNotFound,
	} {
		response, err := http.Get(base + path)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, status, response.StatusCode, path)
	}
	collectGeoIPStatus(t, c)
	reader := c.MetricStore().Read(metrix.ReadRaw())
	got := map[string]float64{}
	for _, name := range []string{"http_success", "http_client_errors", "http_server_errors"} {
		value, ok := reader.Value(name, nil)
		require.True(t, ok, name)
		got[name] = float64(value)
	}
	assert.Equal(t, map[string]float64{
		"http_success":       1,
		"http_client_errors": 2,
		"http_server_errors": 0,
	}, got)
}

func TestRequestCountsByStatusClass(t *testing.T) {
	var counts requestCounts
	for _, status := range []int{200, 204, 304, 400, 404, 499, 500, 503} {
		counts.observe(status)
	}
	assert.Equal(t, []uint64{3, 3, 2}, []uint64{
		counts.success.Load(),
		counts.clientErrors.Load(),
		counts.serverErrors.Load(),
	})
}
