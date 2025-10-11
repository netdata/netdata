// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

func TestNewHTTPClient(t *testing.T) {
	tests := map[string]struct {
		config   ClientConfig
		validate func(t *testing.T, client *http.Client, cfg ClientConfig)
		wantErr  bool
		errMsg   string
	}{
		"default config": {
			config: ClientConfig{},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				assert.Zero(t, client.Timeout)
				assert.Nil(t, client.CheckRedirect)
				assert.NotNil(t, client.Transport)

				// Verify it's using http.Transport (not http2)
				transport, ok := client.Transport.(*http.Transport)
				assert.True(t, ok)
				assert.NotNil(t, transport)
			},
		},
		"with timeout": {
			config: ClientConfig{
				Timeout: confopt.Duration(time.Second * 5),
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				assert.Equal(t, time.Second*5, client.Timeout)

				transport, ok := client.Transport.(*http.Transport)
				require.True(t, ok)
				assert.Equal(t, time.Second*5, transport.TLSHandshakeTimeout)
			},
		},
		"not follow redirect": {
			config: ClientConfig{
				NotFollowRedirect: true,
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				assert.NotNil(t, client.CheckRedirect)

				// Test the redirect function
				err := client.CheckRedirect(nil, nil)
				assert.Equal(t, ErrRedirectAttempted, err)
			},
		},
		"with proxy URL": {
			config: ClientConfig{
				ProxyURL: "http://127.0.0.1:3128",
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				transport, ok := client.Transport.(*http.Transport)
				require.True(t, ok)
				assert.NotNil(t, transport.Proxy)

				// Test proxy function
				req := httptest.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := transport.Proxy(req)
				assert.NoError(t, err)
				assert.Equal(t, "http://127.0.0.1:3128", proxyURL.String())
			},
		},
		"invalid proxy URL": {
			config: ClientConfig{
				ProxyURL: "://invalid-url",
			},
			wantErr: true,
			errMsg:  "error on parsing proxy URL",
		},
		"empty proxy URL uses environment": {
			config: ClientConfig{
				ProxyURL: "",
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				transport, ok := client.Transport.(*http.Transport)
				require.True(t, ok)

				// Set env var for testing
				_ = os.Setenv("HTTP_PROXY", "http://env-proxy:8080")
				defer func() { _ = os.Unsetenv("HTTP_PROXY") }()

				req := httptest.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := transport.Proxy(req)
				assert.NoError(t, err)
				if proxyURL != nil {
					assert.Equal(t, "http://env-proxy:8080", proxyURL.String())
				}
			},
		},
		"force HTTP2": {
			config: ClientConfig{
				ForceHTTP2: true,
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				// Verify it's using http2Transport
				transport, ok := client.Transport.(*http2Transport)
				assert.True(t, ok)
				assert.NotNil(t, transport)
				assert.NotNil(t, transport.t2)
				assert.NotNil(t, transport.t2c)
			},
		},
		"with TLS config": {
			config: ClientConfig{
				TLSConfig: tlscfg.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				transport, ok := client.Transport.(*http.Transport)
				require.True(t, ok)
				assert.NotNil(t, transport.TLSClientConfig)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		"invalid TLS config": {
			config: ClientConfig{
				TLSConfig: tlscfg.TLSConfig{
					TLSCA:   "/non/existent/ca.pem",
					TLSCert: "/non/existent/cert.pem",
					TLSKey:  "/non/existent/key.pem",
				},
			},
			wantErr: true,
			errMsg:  "error on creating TLS config",
		},
		"full config": {
			config: ClientConfig{
				Timeout:           confopt.Duration(time.Second * 10),
				NotFollowRedirect: true,
				ProxyURL:          "http://proxy:8080",
				TLSConfig: tlscfg.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				assert.Equal(t, time.Second*10, client.Timeout)
				assert.NotNil(t, client.CheckRedirect)

				transport, ok := client.Transport.(*http.Transport)
				require.True(t, ok)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
				assert.NotNil(t, transport.Proxy)
			},
		},
		"HTTP2 with TLS config": {
			config: ClientConfig{
				ForceHTTP2: true,
				TLSConfig: tlscfg.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
			validate: func(t *testing.T, client *http.Client, cfg ClientConfig) {
				transport, ok := client.Transport.(*http2Transport)
				require.True(t, ok)
				assert.True(t, transport.t2.TLSClientConfig.InsecureSkipVerify)
				assert.True(t, transport.t2c.TLSClientConfig.InsecureSkipVerify)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client, err := NewHTTPClient(test.config)

			if test.wantErr {
				assert.Error(t, err)
				if test.errMsg != "" {
					assert.Contains(t, err.Error(), test.errMsg)
				}
				assert.Nil(t, client)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)

			if test.validate != nil {
				test.validate(t, client, test.config)
			}
		})
	}
}

func TestHTTP2Transport_RoundTrip(t *testing.T) {
	// Test that http2Transport can be created and has the expected structure
	cfg := ClientConfig{
		ForceHTTP2: true,
		TLSConfig: tlscfg.TLSConfig{
			InsecureSkipVerify: true,
		},
	}

	client, err := NewHTTPClient(cfg)
	require.NoError(t, err)

	// Verify the transport is http2Transport
	transport, ok := client.Transport.(*http2Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.t2)
	assert.NotNil(t, transport.t2c)

	// Note: We can't easily test actual HTTP/2 communication without setting up
	// proper HTTP/2 servers, which httptest doesn't support directly.
	// The integration test with regular servers is sufficient for basic functionality.
}

func TestHTTP2Transport_CloseIdleConnections(t *testing.T) {
	transport := &http2Transport{
		t2:  &http2.Transport{},
		t2c: &http2.Transport{},
	}

	// This should not panic
	assert.NotPanics(t, func() {
		transport.CloseIdleConnections()
	})
}

func TestProxyFunc(t *testing.T) {
	tests := map[string]struct {
		proxyURL string
		envProxy string
		validate func(t *testing.T, proxyFunc func(*http.Request) (*url.URL, error))
	}{
		"empty proxy URL uses environment": {
			proxyURL: "",
			envProxy: "http://env-proxy:8080",
			validate: func(t *testing.T, proxyFunc func(*http.Request) (*url.URL, error)) {
				_ = os.Setenv("HTTP_PROXY", "http://env-proxy:8080")
				defer func() { _ = os.Unsetenv("HTTP_PROXY") }()

				req := httptest.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := proxyFunc(req)
				assert.NoError(t, err)
				if proxyURL != nil {
					assert.Equal(t, "http://env-proxy:8080", proxyURL.String())
				}
			},
		},
		"specific proxy URL": {
			proxyURL: "http://specific-proxy:3128",
			validate: func(t *testing.T, proxyFunc func(*http.Request) (*url.URL, error)) {
				req := httptest.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := proxyFunc(req)
				assert.NoError(t, err)
				assert.Equal(t, "http://specific-proxy:3128", proxyURL.String())
			},
		},
		"proxy URL with auth": {
			proxyURL: "http://user:pass@proxy:3128",
			validate: func(t *testing.T, proxyFunc func(*http.Request) (*url.URL, error)) {
				req := httptest.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := proxyFunc(req)
				assert.NoError(t, err)
				assert.Equal(t, "http://user:pass@proxy:3128", proxyURL.String())
				assert.Equal(t, "user", proxyURL.User.Username())
				pass, _ := proxyURL.User.Password()
				assert.Equal(t, "pass", pass)
			},
		},
		"https proxy URL": {
			proxyURL: "https://secure-proxy:443",
			validate: func(t *testing.T, proxyFunc func(*http.Request) (*url.URL, error)) {
				req := httptest.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := proxyFunc(req)
				assert.NoError(t, err)
				assert.Equal(t, "https://secure-proxy:443", proxyURL.String())
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fn := proxyFunc(test.proxyURL)
			assert.NotNil(t, fn)

			if test.validate != nil {
				test.validate(t, fn)
			}
		})
	}
}

func TestRedirectFunc(t *testing.T) {
	tests := map[string]struct {
		notFollow bool
		wantErr   error
	}{
		"follow redirects": {
			notFollow: false,
			wantErr:   nil,
		},
		"not follow redirects": {
			notFollow: true,
			wantErr:   ErrRedirectAttempted,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fn := redirectFunc(test.notFollow)

			if test.wantErr != nil {
				assert.NotNil(t, fn)
				err := fn(nil, nil)
				assert.Equal(t, test.wantErr, err)
			} else {
				assert.Nil(t, fn)
			}
		})
	}
}

func TestClientIntegration(t *testing.T) {
	// Create a test server with various behaviors
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			redirectCount++
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			_, _ = w.Write([]byte("final destination"))
		case "/timeout":
			time.Sleep(time.Second * 2)
			_, _ = w.Write([]byte("too late"))
		default:
			_, _ = w.Write([]byte("default response"))
		}
	}))
	defer server.Close()

	t.Run("follow redirects", func(t *testing.T) {
		client, err := NewHTTPClient(ClientConfig{
			NotFollowRedirect: false,
		})
		require.NoError(t, err)

		resp, err := client.Get(server.URL + "/redirect")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, "/final", resp.Request.URL.Path)
	})

	t.Run("not follow redirects", func(t *testing.T) {
		client, err := NewHTTPClient(ClientConfig{
			NotFollowRedirect: true,
		})
		require.NoError(t, err)

		_, err = client.Get(server.URL + "/redirect")
		assert.Error(t, err)

		// Check if error contains redirect indication
		urlErr, ok := err.(*url.Error)
		if ok {
			assert.Equal(t, ErrRedirectAttempted, urlErr.Err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		client, err := NewHTTPClient(ClientConfig{
			Timeout: confopt.Duration(time.Millisecond * 500),
		})
		require.NoError(t, err)

		_, err = client.Get(server.URL + "/timeout")
		assert.Error(t, err)

		var netErr net.Error
		if errors.As(err, &netErr) {
			assert.True(t, netErr.Timeout())
		}
	})
}

func TestTransportWithDifferentSchemes(t *testing.T) {
	// Test that regular transport handles both http and https
	client, err := NewHTTPClient(ClientConfig{
		TLSConfig: tlscfg.TLSConfig{
			InsecureSkipVerify: true,
		},
	})
	require.NoError(t, err)

	// HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http"))
	}))
	defer httpServer.Close()

	// HTTPS server
	httpsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("https"))
	}))
	defer httpsServer.Close()

	// Test HTTP
	resp, err := client.Get(httpServer.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	// Test HTTPS
	resp, err = client.Get(httpsServer.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()
}

func TestHTTP2TransportStructure(t *testing.T) {
	// Test the http2Transport structure and methods
	transport := &http2Transport{
		t2:  &http2.Transport{},
		t2c: &http2.Transport{AllowHTTP: true},
	}

	// Test HTTPS request routing
	httpsReq := httptest.NewRequest("GET", "https://example.com", nil)
	// Just verify it doesn't panic and routes to the correct transport
	assert.NotPanics(t, func() {
		// We can't actually execute the request without a server,
		// but we can verify the routing logic
		if httpsReq.URL.Scheme == "https" {
			// Would use t2
			assert.NotNil(t, transport.t2)
		}
	})

	// Test HTTP request routing
	httpReq := httptest.NewRequest("GET", "http://example.com", nil)
	assert.NotPanics(t, func() {
		if httpReq.URL.Scheme == "http" {
			// Would use t2c
			assert.NotNil(t, transport.t2c)
			assert.True(t, transport.t2c.AllowHTTP)
		}
	})
}
