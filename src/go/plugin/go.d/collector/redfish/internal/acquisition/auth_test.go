// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolClientSessionLifecycle(t *testing.T) {
	for _, method := range []string{"session", "auto"} {
		t.Run(method, func(t *testing.T) {
			server := testutil.NewServer(t, testutil.ServerConfig{
				SupportSession:       true,
				MalformedSessionBody: true,
			})
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, method))
			require.NoError(t, client.Check(t.Context()))
			assert.Zero(t, server.SessionCreates.Load())
			for range 2 {
				result, err := client.Acquire(t.Context())
				require.NoError(t, err)
				assert.True(t, result.Complete)
			}
			assert.Equal(t, int64(1), server.SessionCreates.Load())
			client.Close()
			client.Close()
			assert.Equal(t, int64(1), server.SessionDeletes.Load())
			assert.Zero(t, server.ActiveSessions.Load())
		})
	}
}

func TestProtocolClientSessionRetirementDoesNotRestartCanceledContext(t *testing.T) {
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession: true,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	defer client.Close()
	_, err := client.Acquire(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), server.SessionCreates.Load())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client.closeSession(ctx)
	assert.Nil(t, client.sdk)
	assert.Empty(t, client.authMode)
	assert.Zero(t, server.SessionDeletes.Load(), "retirement must not issue a request after cancellation")
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
				testutil.ServeServiceRoot(w, true)
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
				server := testutil.NewServer(t, testutil.ServerConfig{
					RequireBasic: true,
					HandleRequest: func(w http.ResponseWriter, r *http.Request) bool {
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
				result, collectErr := client.Acquire(t.Context())
				if password == "test-password" {
					assert.NoError(t, checkErr)
					require.NoError(t, collectErr)
					assert.True(t, result.Complete)
					assert.True(t, result.Complete)
					assert.NotEmpty(t, result.Resources)
				} else {
					require.Error(t, checkErr)
					require.Error(t, collectErr)
					assert.Equal(t, "auth", classifyError(checkErr))
					assert.False(t, result.Available)
					assert.Equal(t, map[string]int{"auth": 1}, result.Statistics.Failures)
					assert.Equal(t, 1, result.Statistics.HTTPRequests["started"])
					assert.NotContains(t, checkErr.Error(), password)
					assert.NotContains(t, collectErr.Error(), password)
				}
				assert.Zero(t, anonymous.Load())
				assert.Zero(t, server.SessionCreates.Load())
			})
		}
	}
}

func TestProtocolClientReconnectsWithinCycle(t *testing.T) {
	var expire atomic.Bool
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession:      true,
		ExpireSessionOnce:   &expire,
		SessionDeleteStatus: http.StatusServiceUnavailable,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	for cycle := range 3 {
		result, err := client.Acquire(t.Context())
		require.NoError(t, err)
		assert.True(t, result.Complete)
		assert.True(t, result.Complete)
		if cycle == 1 {
			assert.Equal(t, int64(2), server.SessionCreates.Load())
			assert.Equal(t, map[string]int{"auth": 1}, result.Statistics.Failures)
			assert.Equal(t, 1, result.Statistics.Operations["failed"])
		}
		if cycle == 0 {
			expire.Store(true)
			require.NoError(t, client.Check(t.Context()))
			assert.True(t, expire.Load(), "Check does not use or refresh the running session")
		}
	}
	assert.Equal(t, int64(2), server.SessionCreates.Load(), "failed logout does not block a new login")
	client.Close()
}

func TestProtocolClientAutoFallback(t *testing.T) {
	for _, status := range []int{0, 201, 401, 403, 404, 405, 429, 500, 501} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := testutil.NewServer(t, testutil.ServerConfig{
				SessionStatus: status,
				RequireBasic:  true,
			})
			defer server.Close()
			client := newTestProtocolClient(t, testConfig(server.URL, "auto"))
			require.NoError(t, client.Check(t.Context()))
			assert.Zero(t, server.SessionCreates.Load())
			result, err := client.Acquire(t.Context())
			fallback := status == 0 || status == 404 || status == 405 || status == 501
			if fallback {
				require.NoError(t, err)
				assert.True(t, result.Complete)
				assert.Positive(t, server.BasicRequests.Load())
			} else {
				require.Error(t, err)
				assert.False(t, result.Available)
				assert.Zero(t, server.BasicRequests.Load())
			}
			client.Close()
		})
	}
}

func TestProtocolClientSessionDoesNotFallBackToBasic(t *testing.T) {
	server := testutil.NewServer(t, testutil.ServerConfig{
		SessionStatus: http.StatusUnauthorized,
	})
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	require.NoError(t, client.Check(t.Context()))
	result, err := client.Acquire(t.Context())
	require.Error(t, err)
	assert.False(t, result.Available)
	assert.Zero(t, server.BasicRequests.Load())
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
					testutil.ServeServiceRoot(w, true)
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

			result, err := client.Acquire(t.Context())
			require.Error(t, err)
			assert.False(t, result.Available)
			assert.Equal(t, map[string]int{"started": 2, "redirected": 0}, result.Statistics.HTTPRequests)
			assert.Equal(t, map[string]int{"successful": 1, "failed": 1}, result.Statistics.Operations)
			assert.Equal(t, map[string]int{name: 1}, result.Statistics.Failures)
		})
	}
}
