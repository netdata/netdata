// SPDX-License-Identifier: GPL-3.0-or-later

package openmetrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openMetricsResponseBody(t *testing.T, size int64, compressed bool) []byte {
	t.Helper()
	require.Positive(t, size)
	body := bytes.Repeat([]byte("x"), int(size))
	body[0] = '#'
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

func TestResponseLimit(t *testing.T) {
	for name, compressed := range map[string]bool{"plain": false, "gzip": true} {
		t.Run(name, func(t *testing.T) {
			body := openMetricsResponseBody(t, web.DefaultResponseBodyLimit+1, compressed)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			client, err := NewClientWithHTTP(
				Config{
					HTTPConfig: web.HTTPConfig{
						RequestConfig: web.RequestConfig{
							URL: server.URL,
						},
					},
				},
				server.Client(),
			)
			require.NoError(t, err)
			_, err = client.FetchSeries(context.Background(), nil)
			assert.ErrorIs(t, err, web.ErrResponseTooLarge)
		})
	}
}

func TestCustomResponseLimit(t *testing.T) {
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
					cfg := Config{
						HTTPConfig: web.HTTPConfig{
							ClientConfig: web.ClientConfig{
								ResponseBodyLimit: limit,
							},
							RequestConfig: web.RequestConfig{
								URL: server.URL,
							},
						},
					}
					client, err := NewClient(context.Background(), cfg)
					require.NoError(t, err)
					defer client.httpClient.CloseIdleConnections()
					// Build/compress fixtures before starting the client's request deadline.
					body.Store(openMetricsResponseBody(t, limit.Bytes(), compressed))
					_, err = client.FetchSeries(context.Background(), nil)
					require.NoError(t, err, "exact custom limit")
					body.Store(openMetricsResponseBody(t, limit.Bytes()+1, compressed))
					_, err = client.FetchSeries(context.Background(), nil)
					require.ErrorIs(t, err, web.ErrResponseTooLarge)
					body.Store(openMetricsResponseBody(t, 8, compressed))
					_, err = client.FetchSeries(context.Background(), nil)
					require.NoError(t, err, "recovery after overflow")
				})
			}
		})
	}
}
