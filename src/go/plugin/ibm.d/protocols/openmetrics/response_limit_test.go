// SPDX-License-Identifier: GPL-3.0-or-later

package openmetrics

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repeatingByte byte

func (v repeatingByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(v)
	}
	return len(p), nil
}

func TestResponseLimit(t *testing.T) {
	for name, compressed := range map[string]bool{"plain": false, "gzip": true} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var dst io.Writer = w
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
					gz := gzip.NewWriter(w)
					defer gz.Close()
					dst = gz
				}
				_, _ = io.WriteString(dst, "#")
				_, _ = io.CopyN(dst, repeatingByte('x'), web.DefaultResponseBodyLimit)
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
					var size atomic.Int64
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var dst io.Writer = w
						if compressed {
							w.Header().Set("Content-Encoding", "gzip")
							gz := gzip.NewWriter(w)
							defer gz.Close()
							dst = gz
						}
						_, _ = io.WriteString(dst, "#")
						_, _ = io.CopyN(dst, repeatingByte('x'), size.Load()-1)
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
					size.Store(limit.Bytes())
					_, err = client.FetchSeries(context.Background(), nil)
					require.NoError(t, err, "exact custom limit")
					size.Store(limit.Bytes() + 1)
					_, err = client.FetchSeries(context.Background(), nil)
					require.ErrorIs(t, err, web.ErrResponseTooLarge)
					size.Store(8)
					_, err = client.FetchSeries(context.Background(), nil)
					require.NoError(t, err, "recovery after overflow")
				})
			}
		})
	}
}
