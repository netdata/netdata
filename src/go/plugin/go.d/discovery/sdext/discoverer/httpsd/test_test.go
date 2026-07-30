// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ dyncfg.Testable = (*Discoverer)(nil)

func TestDiscovererTestUnsupportedMethodsDoNoOperationalWork(t *testing.T) {
	for _, method := range []string{"get", http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "CUSTOM"} {
		t.Run(method, func(t *testing.T) {
			d, err := NewDiscoverer(Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:             "http://127.0.0.1/discovery",
						Method:          method,
						BearerTokenFile: t.TempDir() + "/missing-token",
					},
				},
			})
			require.NoError(t, err)

			transport := &testTransport{}
			d.client.Transport = transport

			err = d.Test(t.Context())

			require.ErrorIs(t, err, dyncfg.ErrTestUnsupported)
			assert.Zero(t, transport.roundTrips.Load())
		})
	}
}

func TestDiscovererTestUsesConfiguredGETRequest(t *testing.T) {
	for _, method := range []string{"", http.MethodGet} {
		name := method
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			tokenFile := t.TempDir() + "/token"
			require.NoError(t, os.WriteFile(tokenFile, []byte("test-token"), 0o600))

			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "test-value", r.Header.Get("X-Test"))
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `[{"name":"api","url":"http://127.0.0.1"}]`)
			}))
			defer srv.Close()

			d, err := NewDiscoverer(Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:             srv.URL,
						Method:          method,
						BearerTokenFile: tokenFile,
						Headers:         map[string]string{"X-Test": "test-value"},
					},
				},
			})
			require.NoError(t, err)

			require.NoError(t, d.Test(t.Context()))
			assert.EqualValues(t, 1, requests.Load())
		})
	}
}

func TestDiscovererTestPublicErrors(t *testing.T) {
	t.Run("request creation", func(t *testing.T) {
		tokenFile := t.TempDir() + "/empty-token"
		require.NoError(t, os.WriteFile(tokenFile, nil, 0o600))
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:             "http://127.0.0.1/discovery",
				BearerTokenFile: tokenFile,
			},
		})

		err := d.Test(t.Context())

		requirePublicError(t, err, "the configured HTTP discovery request could not be created")
	})

	t.Run("credential file", func(t *testing.T) {
		path := t.TempDir() + "/missing-token"
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:             "http://127.0.0.1/discovery",
				BearerTokenFile: path,
			},
		})

		err := d.Test(t.Context())

		requirePublicError(t, err, "the configured HTTP credential or TLS file could not be read safely")
		require.ErrorIs(t, err, safefile.ErrFile)
		assert.NotContains(t, err.Error(), path)
	})

	t.Run("TLS file", func(t *testing.T) {
		path := t.TempDir() + "/missing-ca"
		_, err := NewDiscoverer(Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{URL: "https://127.0.0.1/discovery"},
				ClientConfig: web.ClientConfig{
					TLSConfig: tlscfg.TLSConfig{TLSCA: path},
				},
			},
		})

		requirePublicError(t, err, "the configured HTTP credential or TLS file could not be read safely")
		require.ErrorIs(t, err, tlscfg.ErrTLSFile)
		require.ErrorIs(t, err, safefile.ErrFile)
		assert.NotContains(t, err.Error(), path)
	})

	t.Run("query", func(t *testing.T) {
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: "http://private.example.invalid/discovery"},
		})
		d.client.Transport = &testTransport{err: errors.New("private transport response")}

		err := d.Test(t.Context())

		requirePublicError(t, err, "cannot query the configured HTTP endpoint")
		assert.NotContains(t, err.Error(), "private")
	})

	t.Run("status", func(t *testing.T) {
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: "http://private.example.invalid/discovery"},
		})
		d.client.Transport = &testTransport{
			statusCode: http.StatusUnauthorized,
			body:       "private response body",
		}

		err := d.Test(t.Context())

		requirePublicError(t, err, "cannot query the configured HTTP endpoint")
		assert.NotContains(t, err.Error(), "private")
	})

	t.Run("invalid response", func(t *testing.T) {
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: "http://private.example.invalid/discovery"},
		})
		d.client.Transport = &testTransport{
			statusCode:  http.StatusOK,
			contentType: "application/json",
			body:        "[private invalid response",
		}

		err := d.Test(t.Context())

		requirePublicError(t, err, "the configured HTTP endpoint did not return usable discovery data")
		assert.NotContains(t, err.Error(), "private")
	})

	t.Run("oversized response", func(t *testing.T) {
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: "http://127.0.0.1/discovery"},
		})
		d.client.Transport = &testTransport{
			statusCode: http.StatusOK,
			body:       strings.Repeat("x", responseBodyLimit+1),
		}

		err := d.Test(t.Context())

		requirePublicError(t, err, "the configured HTTP endpoint did not return usable discovery data")
	})

	t.Run("configured timeout", func(t *testing.T) {
		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: "http://127.0.0.1/discovery"},
			ClientConfig:  web.ClientConfig{Timeout: confopt.Duration(20 * time.Millisecond)},
		})
		d.client.Transport = &testTransport{waitForContext: true}

		err := d.Test(t.Context())

		requirePublicError(t, err, "the configured HTTP endpoint did not respond before the timeout")
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestDiscovererTestUsesConfiguredTLSAndProxyPaths(t *testing.T) {
	t.Run("TLS CA", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `[]`)
		}))
		defer srv.Close()

		caFile := t.TempDir() + "/ca.pem"
		caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
		require.NoError(t, os.WriteFile(caFile, caPEM, 0o600))

		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
			ClientConfig: web.ClientConfig{
				TLSConfig: tlscfg.TLSConfig{TLSCA: caFile},
			},
		})

		require.NoError(t, d.Test(t.Context()))
	})

	t.Run("proxy", func(t *testing.T) {
		var requests atomic.Int64
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			assert.Equal(t, "origin.example.invalid", r.URL.Host)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `[]`)
		}))
		defer proxy.Close()

		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: "http://origin.example.invalid/discovery"},
			ClientConfig:  web.ClientConfig{ProxyURL: proxy.URL},
		})

		require.NoError(t, d.Test(t.Context()))
		assert.EqualValues(t, 1, requests.Load())
	})
}

func TestDiscovererTestRedirectRequestBound(t *testing.T) {
	t.Run("ten requests can succeed", func(t *testing.T) {
		var requests atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := requests.Add(1)
			if n < 10 {
				http.Redirect(w, r, fmt.Sprintf("/%d", n), http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `[]`)
		}))
		defer srv.Close()

		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
		})

		require.NoError(t, d.Test(t.Context()))
		assert.EqualValues(t, 10, requests.Load())
	})

	t.Run("eleventh request is not made", func(t *testing.T) {
		var requests atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := requests.Add(1)
			http.Redirect(w, r, fmt.Sprintf("/%d", n), http.StatusFound)
		}))
		defer srv.Close()

		d := newTestDiscoverer(t, web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
		})

		err := d.Test(t.Context())

		requirePublicError(t, err, "cannot query the configured HTTP endpoint")
		assert.EqualValues(t, 10, requests.Load())
	})
}

func TestDiscovererTestClosesResponseAndIdleConnections(t *testing.T) {
	body := &testBody{Reader: strings.NewReader("[]")}
	transport := &testTransport{
		statusCode:   http.StatusOK,
		contentType:  "application/json",
		responseBody: body,
	}
	d := newTestDiscoverer(t, web.HTTPConfig{
		RequestConfig: web.RequestConfig{URL: "http://127.0.0.1/discovery"},
	})
	d.client.Transport = transport

	require.NoError(t, d.Test(t.Context()))
	assert.True(t, body.closed.Load())
	assert.True(t, transport.idleClosed.Load())
}

func TestDiscovererTestHonorsCallerCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	transport := &testTransport{
		waitForContext: true,
		started:        started,
	}
	d := newTestDiscoverer(t, web.HTTPConfig{
		RequestConfig: web.RequestConfig{URL: "http://127.0.0.1/discovery"},
		ClientConfig:  web.ClientConfig{Timeout: confopt.Duration(200 * time.Millisecond)},
	})
	d.client.Transport = transport

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- d.Test(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("HTTP discovery test did not start its request")
	}
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
		assert.True(t, transport.idleClosed.Load())
	case <-time.After(time.Second):
		t.Fatal("HTTP discovery test did not honor caller cancellation")
	}
}

func newTestDiscoverer(t *testing.T, cfg web.HTTPConfig) *Discoverer {
	t.Helper()
	d, err := NewDiscoverer(Config{HTTPConfig: cfg})
	require.NoError(t, err)
	return d
}

func requirePublicError(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, want, err.Error())
	message, ok := dyncfg.PublicMessage(err)
	require.True(t, ok)
	assert.Equal(t, want, message)
}

type testTransport struct {
	roundTrips     atomic.Int64
	idleClosed     atomic.Bool
	statusCode     int
	contentType    string
	body           string
	responseBody   io.ReadCloser
	err            error
	waitForContext bool
	started        chan struct{}
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.roundTrips.Add(1)
	if t.started != nil {
		select {
		case t.started <- struct{}{}:
		default:
		}
	}
	if t.waitForContext {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	if t.err != nil {
		return nil, t.err
	}
	statusCode := t.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	body := t.responseBody
	if body == nil {
		body = io.NopCloser(strings.NewReader(t.body))
	}
	header := make(http.Header)
	if t.contentType != "" {
		header.Set("Content-Type", t.contentType)
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       body,
		Request:    req,
	}, nil
}

func (t *testTransport) CloseIdleConnections() {
	t.idleClosed.Store(true)
}

type testBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *testBody) Close() error {
	b.closed.Store(true)
	return nil
}
