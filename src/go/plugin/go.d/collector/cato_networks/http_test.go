// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func catoResponseBody(t *testing.T, payload string, size int64, compressed bool) []byte {
	t.Helper()
	require.GreaterOrEqual(t, size, int64(len(payload)))
	body := bytes.Repeat([]byte(" "), int(size))
	copy(body[len(body)-len(payload):], payload)
	if !compressed {
		return body
	}
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	require.NoError(t, err)
	_, err = gz.Write(body)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestCatoResponseLimit(t *testing.T) {
	for name, compressed := range map[string]bool{"plain": false, "explicit gzip": true} {
		t.Run(name, func(t *testing.T) {
			body := catoResponseBody(t, `{"data":{}}`, web.DefaultResponseBodyLimit+1, compressed)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
				}
				// One write avoids tiny chunks repeatedly refilling a large JSON decoder buffer under -race.
				_, _ = w.Write(body)
			}))
			defer server.Close()
			cfg := New().Config
			cfg.URL = server.URL
			cfg.APIKey = "test-key"
			cfg.AccountID = "123"
			cfg.Headers = map[string]string{"Accept-Encoding": "gzip"}
			client, err := newSDKAPIClient(cfg, server.Client())
			require.NoError(t, err)
			t.Run("SDK", func(t *testing.T) {
				assert.ErrorIs(t, client.Probe(context.Background(), cfg.AccountID), web.ErrResponseTooLarge)
			})
			t.Run("raw", func(t *testing.T) {
				_, err := client.AccountSnapshot(context.Background(), cfg.AccountID, nil)
				assert.ErrorIs(t, err, web.ErrResponseTooLarge)
			})
		})
	}
}

func TestCatoHTTPClientPreservesClientPolicy(t *testing.T) {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	redirectErr := io.ErrUnexpectedEOF
	original := &http.Client{
		Timeout:       time.Second,
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return redirectErr },
	}
	cloned := newCatoHTTPClient(original)
	assert.Nil(t, original.Transport)
	assert.Equal(t, original.Timeout, cloned.Timeout)
	assert.Same(t, jar, cloned.Jar)
	assert.ErrorIs(t, cloned.CheckRedirect(nil, nil), redirectErr)
}

type catoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f catoRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type catoTestBody struct {
	io.Reader
	closed bool
}

func (b *catoTestBody) Close() error { b.closed = true; return nil }

func TestCatoGzipBodyLifecycle(t *testing.T) {
	var payload bytes.Buffer
	gz := gzip.NewWriter(&payload)
	_, err := io.WriteString(gz, `{"ok":true}`)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	for name, tc := range map[string]struct {
		data    string
		wantErr bool
	}{
		"valid":          {data: payload.String()},
		"invalid header": {data: "invalid", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			body := &catoTestBody{
				Reader: strings.NewReader(tc.data),
			}
			client := newCatoHTTPClient(
				&http.Client{
					Transport: catoRoundTripFunc(func(*http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Encoding": {"gzip"},
								"Content-Length":   {"123"},
								"Trace_id":         {"test-trace"},
							},
							Body: body,
						}, nil
					}),
				},
			)
			resp, err := client.Get("http://example.com")
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, body.closed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "", resp.Header.Get("Content-Encoding"))
			assert.Equal(t, "", resp.Header.Get("Content-Length"))
			assert.Equal(t, []string{"test-trace"}, resp.Header["Trace_id"])
			assert.True(t, resp.Uncompressed)
			assert.EqualValues(t, -1, resp.ContentLength)
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, `{"ok":true}`, string(got))
			require.NoError(t, resp.Body.Close())
			assert.True(t, body.closed)
		})
	}
}

func TestCatoCustomResponseLimit(t *testing.T) {
	for name, limit := range map[string]confopt.ByteSize{"smaller": confopt.KiB, "larger": 64*confopt.MiB + confopt.KiB} {
		t.Run(name, func(t *testing.T) {
			for encoding, compressed := range map[string]bool{"plain": false, "gzip": true} {
				t.Run(encoding, func(t *testing.T) {
					var body atomic.Value
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if compressed {
							w.Header().Set("Content-Encoding", "gzip")
						}
						_, _ = w.Write(body.Load().([]byte))
					}))
					defer server.Close()
					cfg := New().Config
					cfg.URL = server.URL
					cfg.APIKey = "test-key"
					cfg.AccountID = "123"
					cfg.Headers = map[string]string{"Accept-Encoding": "gzip"}
					base, err := web.NewHTTPClient(context.Background(), web.ClientConfig{
						ResponseBodyLimit: limit,
					})
					require.NoError(t, err)
					defer base.CloseIdleConnections()
					assert.Equal(t, limit.Bytes(), web.HTTPClientResponseBodyLimit(newCatoHTTPClient(base)))
					client, err := newSDKAPIClient(cfg, base)
					require.NoError(t, err)
					for path, fetch := range map[string]func() error{
						"SDK": func() error { return client.Probe(context.Background(), cfg.AccountID) },
						"raw": func() error { _, err := client.AccountSnapshot(context.Background(), cfg.AccountID, nil); return err },
					} {
						t.Run(path, func(t *testing.T) {
							payload := `{"data":{"accountSnapshot":{"sites":[]}}}`
							if path == "SDK" {
								payload = `{"data":{"entityLookup":{"items":[]}}}`
							}
							body.Store(catoResponseBody(t, payload, limit.Bytes(), compressed))
							require.NoError(t, fetch(), "exact custom limit")
							body.Store(catoResponseBody(t, payload, limit.Bytes()+1, compressed))
							require.ErrorIs(t, fetch(), web.ErrResponseTooLarge)
							body.Store(catoResponseBody(t, payload, 64, compressed))
							require.NoError(t, fetch(), "recovery after overflow")
						})
					}
				})
			}
		})
	}
}
