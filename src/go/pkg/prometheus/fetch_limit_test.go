// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type repeatingByte byte

func (v repeatingByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(v)
	}
	return len(p), nil
}

type fetchRoundTripFunc func(*http.Request) (*http.Response, error)

func (f fetchRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fetchBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *fetchBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *fetchBody) Close() error { b.closed = true; return nil }

func TestHTTPFetchBoundsCleanup(t *testing.T) {
	for name, status := range map[string]int{"HTTP error": http.StatusBadGateway, "invalid gzip": http.StatusOK} {
		t.Run(name, func(t *testing.T) {
			body := &fetchBody{
				Reader: io.LimitReader(repeatingByte('x'), web.DefaultResponseBodyLimit+4096),
			}
			client := &http.Client{
				Transport: fetchRoundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: status,
						Header: http.Header{
							"Content-Encoding": {"gzip"},
						},
						Body: body,
					}, nil
				}),
			}
			f := httpFetcher{
				client: client,
				request: web.RequestConfig{
					URL: "http://example.com",
				},
			}
			require.Error(t, f.fetch(context.Background(), io.Discard))
			assert.True(t, body.closed)
			assert.Equal(t, web.DefaultResponseBodyLimit+1, body.read)
		})
	}
}

func TestPrometheusResponseLimit(t *testing.T) {
	const limit = 64 << 20
	for name, compressed := range map[string]bool{"plain": false, "gzip": true} {
		t.Run(name, func(t *testing.T) {
			var oversized atomic.Bool
			oversized.Store(true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var dst io.Writer = w
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
					gz := gzip.NewWriter(w)
					defer gz.Close()
					dst = gz
				}
				if oversized.Load() {
					_, _ = io.WriteString(dst, "#")
					_, _ = io.CopyN(dst, repeatingByte('x'), limit)
				} else {
					_, _ = io.WriteString(dst, "metric 1\n")
				}
			}))
			defer server.Close()
			p := New(server.Client(), web.RequestConfig{
				URL: server.URL,
			}).(*prometheus)
			for entry, scrape := range map[string]func() error{
				"samples":  func() error { _, err := p.ScrapeSamples(context.Background()); return err },
				"series":   func() error { _, err := p.ScrapeSeries(context.Background()); return err },
				"families": func() error { _, err := p.ScrapeContext(context.Background()); return err },
			} {
				t.Run(entry, func(t *testing.T) {
					oversized.Store(true)
					assert.ErrorIs(t, scrape(), web.ErrResponseTooLarge)
					assert.Equal(t, limit, p.buf.Len())
					assert.LessOrEqual(t, p.buf.Cap(), 2*limit)
					oversized.Store(false)
					require.NoError(t, scrape())
					assert.Equal(t, "metric 1\n", p.buf.String())
				})
			}
		})
	}
}

func BenchmarkHTTPFetch(b *testing.B) {
	for name, compressed := range map[string]bool{"plain": false, "gzip": true} {
		b.Run(name, func(b *testing.B) {
			payload := bytes.Repeat([]byte("metric 1\n"), 8192)
			if compressed {
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				_, _ = gz.Write(payload)
				_ = gz.Close()
				payload = buf.Bytes()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
				}
				_, _ = w.Write(payload)
			}))
			defer server.Close()
			client, err := web.NewHTTPClient(context.Background(), web.ClientConfig{})
			require.NoError(b, err)
			defer client.CloseIdleConnections()
			f := httpFetcher{
				client: client,
				request: web.RequestConfig{
					URL: server.URL,
				},
			}
			var buf bytes.Buffer
			require.NoError(b, f.fetch(context.Background(), &buf))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				buf.Reset()
				require.NoError(b, f.fetch(context.Background(), &buf))
			}
		})
	}
}

func TestPrometheusCustomResponseLimit(t *testing.T) {
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
					client, err := web.NewHTTPClient(context.Background(), web.ClientConfig{
						ResponseBodyLimit: limit,
					})
					require.NoError(t, err)
					defer client.CloseIdleConnections()
					p := NewWithSelector(client, web.RequestConfig{
						URL: server.URL,
					}, nil)
					size.Store(limit.Bytes())
					_, err = p.ScrapeSeries(context.Background())
					require.NoError(t, err, "exact custom limit")
					size.Store(limit.Bytes() + 1)
					_, err = p.ScrapeSeries(context.Background())
					require.ErrorIs(t, err, web.ErrResponseTooLarge)
					size.Store(8)
					_, err = p.ScrapeSeries(context.Background())
					require.NoError(t, err, "recovery after overflow")
				})
			}
		})
	}
}

func TestPrometheusLimitFollowsHTTPClient(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "metric 1\n") }),
	)
	defer server.Close()
	first, err := web.NewHTTPClient(context.Background(), web.ClientConfig{
		ResponseBodyLimit: 4,
	})
	require.NoError(t, err)
	defer first.CloseIdleConnections()
	p := New(first, web.RequestConfig{
		URL: server.URL,
	})
	_, err = p.ScrapeSeries(context.Background())
	require.ErrorIs(t, err, web.ErrResponseTooLarge)
	second, err := web.NewHTTPClient(context.Background(), web.ClientConfig{
		ResponseBodyLimit: 16,
	})
	require.NoError(t, err)
	defer second.CloseIdleConnections()
	first.CloseIdleConnections()
	p.HTTPClient().Transport = second.Transport
	_, err = p.ScrapeSeries(context.Background())
	require.NoError(t, err)
}
