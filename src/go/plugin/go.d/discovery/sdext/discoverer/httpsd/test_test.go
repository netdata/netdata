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
	tests := map[string]struct {
		method string
	}{
		"lowercase GET": {method: "get"},
		"POST":          {method: http.MethodPost},
		"PUT":           {method: http.MethodPut},
		"PATCH":         {method: http.MethodPatch},
		"DELETE":        {method: http.MethodDelete},
		"custom":        {method: "CUSTOM"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			d, err := NewDiscoverer(Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:             "http://127.0.0.1/discovery",
						Method:          tc.method,
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
	tests := map[string]struct {
		method string
	}{
		"default": {},
		"GET":     {method: http.MethodGet},
	}

	for name, tc := range tests {
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
						Method:          tc.method,
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
	tests := map[string]struct {
		run         func(t *testing.T) (error, []string)
		wantMessage string
		wantErrs    []error
	}{
		"request creation": {
			run: func(t *testing.T) (error, []string) {
				tokenFile := t.TempDir() + "/empty-token"
				require.NoError(t, os.WriteFile(tokenFile, nil, 0o600))
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:             "http://127.0.0.1/discovery",
						BearerTokenFile: tokenFile,
					},
				})
				return d.Test(t.Context()), nil
			},
			wantMessage: "the configured HTTP discovery request could not be created",
		},
		"credential file": {
			run: func(t *testing.T) (error, []string) {
				path := t.TempDir() + "/missing-token"
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:             "http://127.0.0.1/discovery",
						BearerTokenFile: path,
					},
				})
				return d.Test(t.Context()), []string{path}
			},
			wantMessage: "the configured HTTP credential or TLS file could not be read safely",
			wantErrs:    []error{safefile.ErrFile},
		},
		"TLS file": {
			run: func(t *testing.T) (error, []string) {
				path := t.TempDir() + "/missing-ca"
				_, err := NewDiscoverer(Config{
					HTTPConfig: web.HTTPConfig{
						RequestConfig: web.RequestConfig{URL: "https://127.0.0.1/discovery"},
						ClientConfig: web.ClientConfig{
							TLSConfig: tlscfg.TLSConfig{TLSCA: path},
						},
					},
				})
				return err, []string{path}
			},
			wantMessage: "the configured HTTP credential or TLS file could not be read safely",
			wantErrs:    []error{tlscfg.ErrTLSFile, safefile.ErrFile},
		},
		"query": {
			run: func(t *testing.T) (error, []string) {
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://private.example.invalid/discovery"},
				})
				d.client.Transport = &testTransport{err: errors.New("private transport response")}
				return d.Test(t.Context()), []string{"private"}
			},
			wantMessage: "cannot query the configured HTTP endpoint",
		},
		"status": {
			run: func(t *testing.T) (error, []string) {
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://private.example.invalid/discovery"},
				})
				d.client.Transport = &testTransport{
					statusCode: http.StatusUnauthorized,
					body:       "private response body",
				}
				return d.Test(t.Context()), []string{"private"}
			},
			wantMessage: "cannot query the configured HTTP endpoint",
		},
		"invalid response": {
			run: func(t *testing.T) (error, []string) {
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://private.example.invalid/discovery"},
				})
				d.client.Transport = &testTransport{
					statusCode:  http.StatusOK,
					contentType: "application/json",
					body:        "[private invalid response",
				}
				return d.Test(t.Context()), []string{"private"}
			},
			wantMessage: "the configured HTTP endpoint did not return usable discovery data",
		},
		"oversized response": {
			run: func(t *testing.T) (error, []string) {
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1/discovery"},
				})
				d.client.Transport = &testTransport{
					statusCode: http.StatusOK,
					body:       strings.Repeat("x", responseBodyLimit+1),
				}
				return d.Test(t.Context()), nil
			},
			wantMessage: "the configured HTTP endpoint did not return usable discovery data",
		},
		"configured timeout": {
			run: func(t *testing.T) (error, []string) {
				d := newTestDiscoverer(t, web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1/discovery"},
					ClientConfig:  web.ClientConfig{Timeout: confopt.Duration(20 * time.Millisecond)},
				})
				d.client.Transport = &testTransport{waitForContext: true}
				return d.Test(t.Context()), nil
			},
			wantMessage: "the configured HTTP endpoint did not respond before the timeout",
			wantErrs:    []error{context.DeadlineExceeded},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err, privateValues := tc.run(t)

			requirePublicError(t, err, tc.wantMessage)
			for _, wantErr := range tc.wantErrs {
				require.ErrorIs(t, err, wantErr)
			}
			for _, privateValue := range privateValues {
				assert.NotContains(t, err.Error(), privateValue)
			}
		})
	}
}

func TestDiscovererTestUsesConfiguredTLSAndProxyPaths(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T) error
	}{
		"TLS CA": {
			run: func(t *testing.T) error {
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
				return d.Test(t.Context())
			},
		},
		"proxy": {
			run: func(t *testing.T) error {
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
				err := d.Test(t.Context())
				assert.EqualValues(t, 1, requests.Load())
				return err
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, tc.run(t))
		})
	}
}

func TestDiscovererTestRedirectRequestBound(t *testing.T) {
	tests := map[string]struct {
		successAt   int64
		wantMessage string
	}{
		"ten requests can succeed": {
			successAt: 10,
		},
		"eleventh request is not made": {
			wantMessage: "cannot query the configured HTTP endpoint",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				if tc.successAt > 0 && n >= tc.successAt {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `[]`)
					return
				}
				http.Redirect(w, r, fmt.Sprintf("/%d", n), http.StatusFound)
			}))
			defer srv.Close()

			d := newTestDiscoverer(t, web.HTTPConfig{
				RequestConfig: web.RequestConfig{URL: srv.URL},
			})

			err := d.Test(t.Context())

			if tc.wantMessage == "" {
				require.NoError(t, err)
			} else {
				requirePublicError(t, err, tc.wantMessage)
			}
			assert.EqualValues(t, 10, requests.Load())
		})
	}
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
