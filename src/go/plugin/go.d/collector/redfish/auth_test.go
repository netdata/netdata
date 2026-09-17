// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolClientSessionLifecycle(t *testing.T) {
	for _, method := range []string{"session", "auto"} {
		t.Run(method, func(t *testing.T) {
			server := newRedfishTestServer(t, redfishTestServerConfig{
				supportSession:       true,
				malformedSessionBody: true,
			})
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, method))
			require.NoError(t, client.Check(t.Context()))
			assert.Zero(t, server.sessionCreates.Load())
			for range 2 {
				result, err := client.Collect(t.Context())
				require.NoError(t, err)
				assert.True(t, result.Complete)
			}
			assert.Equal(t, int64(1), server.sessionCreates.Load())
			client.Close()
			client.Close()
			assert.Equal(t, int64(1), server.sessionDeletes.Load())
			assert.Zero(t, server.activeSessions.Load())
		})
	}
}

func TestProtocolClientCheckOnlyReadsServiceRoot(t *testing.T) {
	for _, method := range []string{"auto", "session", "basic", "none"} {
		t.Run(method, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/redfish/v1/", r.URL.Path)
				assert.Empty(t, r.Header.Get("X-Auth-Token"))
				serveServiceRoot(w, true)
			}))
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, method))
			require.NoError(t, client.Check(t.Context()))
			client.Close()
			assert.Positive(t, requests.Load())
			assert.Nil(t, client.sdk)
		})
	}
}

func TestProtocolClientReconnectsOnNextCycle(t *testing.T) {
	var expire atomic.Bool
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession:      true,
		expireSessionOnce:   &expire,
		sessionDeleteStatus: http.StatusServiceUnavailable,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	for cycle := range 3 {
		result, err := client.Collect(t.Context())
		if cycle == 1 {
			require.Error(t, err)
			assert.Equal(t, "unavailable", result.Metrics.Status)
			assert.Equal(t, int64(1), server.sessionCreates.Load(), "no refresh/replay within the failed cycle")
		} else {
			require.NoError(t, err)
			assert.True(t, result.Complete)
		}
		if cycle == 0 {
			expire.Store(true)
			require.NoError(t, client.Check(t.Context()))
			assert.True(t, expire.Load(), "Check does not use or refresh the running session")
		}
	}
	assert.Equal(t, int64(2), server.sessionCreates.Load(), "failed logout does not block a new login")
	client.Close()
}

func TestProtocolClientAutoFallback(t *testing.T) {
	for _, status := range []int{0, 201, 401, 403, 404, 405, 429, 500, 501} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := newRedfishTestServer(t, redfishTestServerConfig{
				sessionStatus: status,
				requireBasic:  true,
			})
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, "auto"))
			require.NoError(t, client.Check(t.Context()))
			assert.Zero(t, server.sessionCreates.Load())
			result, err := client.Collect(t.Context())
			fallback := status == 0 || status == 404 || status == 405 || status == 501
			if fallback {
				require.NoError(t, err)
				assert.True(t, result.Complete)
				assert.Positive(t, server.basicRequests.Load())
			} else {
				require.Error(t, err)
				assert.Equal(t, "unavailable", result.Metrics.Status)
				assert.Zero(t, server.basicRequests.Load())
			}
			client.Close()
		})
	}
}

func TestProtocolClientSessionDoesNotFallBackToBasic(t *testing.T) {
	server := newRedfishTestServer(t, redfishTestServerConfig{
		sessionStatus: http.StatusUnauthorized,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(t.Context()))
	result, err := client.Collect(t.Context())
	require.Error(t, err)
	assert.Equal(t, "unavailable", result.Metrics.Status)
	assert.Zero(t, server.basicRequests.Load())
}
