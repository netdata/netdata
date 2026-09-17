// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

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

func TestProtocolClientSessionRetirementDoesNotRestartCanceledContext(t *testing.T) {
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	defer client.Close()
	_, err := client.Collect(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), server.sessionCreates.Load())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client.closeSession(ctx)
	assert.Nil(t, client.sdk)
	assert.Empty(t, client.authMode)
	assert.Zero(t, server.sessionDeletes.Load(), "retirement must not issue a request after cancellation")
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
				if method == "basic" {
					username, password, ok := r.BasicAuth()
					assert.True(t, ok)
					assert.Equal(t, "user", username)
					assert.Equal(t, "test-password", password)
				} else {
					assert.Empty(t, r.Header.Get("Authorization"))
				}
				serveServiceRoot(w, true)
			}))
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, method))
			require.NoError(t, client.Check(t.Context()))
			client.Close()
			assert.Equal(t, int64(1), requests.Load())
			assert.Nil(t, client.sdk)
		})
	}
}

func TestProtocolClientBasicProtectedServiceRoot(t *testing.T) {
	// Some services require credentials even at the root; see DMTF/python-redfish-library#165.
	for _, redirect := range []bool{false, true} {
		for _, password := range []string{"test-password", "wrong-password"} {
			t.Run(fmt.Sprintf("redirect=%v/password=%s", redirect, password), func(t *testing.T) {
				var anonymous atomic.Int64
				server := newRedfishTestServer(t, redfishTestServerConfig{
					requireBasic: true,
					handleRequest: func(w http.ResponseWriter, r *http.Request) bool {
						username, secret, ok := r.BasicAuth()
						if !ok {
							anonymous.Add(1)
						}
						if !ok || username != "user" || secret != "test-password" {
							http.Error(w, "unauthorized", http.StatusUnauthorized)
							return true
						}
						if redirect && r.URL.Path == "/redfish/v1/" {
							http.Redirect(w, r, "/redfish/v1/redirected", http.StatusTemporaryRedirect)
							return true
						}
						return false
					},
				})
				defer server.Close()
				cfg := testConfig(server.URL, "basic")
				cfg.Password = password
				client := newTestProtocolClient(t, cfg)
				checkErr := client.Check(t.Context())
				result, collectErr := client.Collect(t.Context())
				if password == "test-password" {
					assert.NoError(t, checkErr)
					require.NoError(t, collectErr)
					assert.True(t, result.Complete)
					assert.Equal(t, "success", result.Metrics.Status)
					assert.NotEmpty(t, result.Hardware)
				} else {
					require.Error(t, checkErr)
					require.Error(t, collectErr)
					assert.Equal(t, "auth", classifyError(checkErr))
					assert.Equal(t, "unavailable", result.Metrics.Status)
					assert.Equal(t, map[string]int{"auth": 1}, result.Metrics.Failures)
					assert.Equal(t, 1, result.Metrics.HTTPRequests["started"])
					assert.NotContains(t, checkErr.Error(), password)
					assert.NotContains(t, collectErr.Error(), password)
				}
				assert.Zero(t, anonymous.Load())
				assert.Zero(t, server.sessionCreates.Load())
			})
		}
	}
}

func TestProtocolClientReconnectsWithinCycle(t *testing.T) {
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
		require.NoError(t, err)
		assert.True(t, result.Complete)
		assert.Equal(t, "success", result.Metrics.Status)
		if cycle == 1 {
			assert.Equal(t, int64(2), server.sessionCreates.Load())
			assert.Equal(t, map[string]int{"auth": 1}, result.Metrics.Failures)
			assert.Equal(t, 1, result.Metrics.Operations["failed"])
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

func TestProtocolClientSessionTransportFailureMetrics(t *testing.T) {
	for name, fail := range map[string]http.HandlerFunc{
		"transport": func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		},
		"timeout": func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					serveServiceRoot(w, true)
					return
				}
				assert.Equal(t, http.MethodPost, r.Method)
				_, _ = io.Copy(io.Discard, r.Body)
				fail(w, r)
			}))
			defer server.Close()
			cfg := testConfig(server.URL, "session")
			cfg.Timeout = confopt.Duration(time.Second)
			client := newTestProtocolClient(t, cfg)
			defer client.Close()

			result, err := client.Collect(t.Context())
			require.Error(t, err)
			assert.Equal(t, "unavailable", result.Metrics.Status)
			assert.Equal(t, map[string]int{"started": 2, "redirected": 0}, result.Metrics.HTTPRequests)
			assert.Equal(t, map[string]int{"successful": 1, "failed": 1}, result.Metrics.Operations)
			assert.Equal(t, map[string]int{name: 1}, result.Metrics.Failures)
		})
	}
}
