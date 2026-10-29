// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"gopkg.in/yaml.v2"
)

type repeatingByte byte

func (v repeatingByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(v)
	}
	return len(p), nil
}

func TestHTTPClientResponseLimit(t *testing.T) {
	const limit = 64 << 20
	for name, tc := range map[string]struct {
		size  int64
		limit confopt.ByteSize
		gzip  bool
	}{
		"exact limit":         {size: limit},
		"plain overflow":      {size: limit + 1},
		"gzip overflow":       {size: limit + 1, gzip: true},
		"negative default":    {size: limit + 1, limit: -1},
		"small exact":         {size: 1024, limit: confopt.KiB},
		"small overflow":      {size: 1025, limit: confopt.KiB},
		"small gzip overflow": {size: 1025, limit: confopt.KiB, gzip: true},
		"large exact":         {size: limit + 1024, limit: 64*confopt.MiB + confopt.KiB},
		"large gzip exact":    {size: limit + 1024, limit: 64*confopt.MiB + confopt.KiB, gzip: true},
		"large overflow":      {size: limit + 1025, limit: 64*confopt.MiB + confopt.KiB},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var dst io.Writer = w
				if tc.gzip {
					w.Header().Set("Content-Encoding", "gzip")
					gz := gzip.NewWriter(w)
					defer gz.Close()
					dst = gz
				}
				_, _ = io.CopyN(dst, repeatingByte('x'), tc.size)
			}))
			defer server.Close()
			client, err := NewHTTPClient(context.Background(), ClientConfig{
				ResponseBodyLimit: tc.limit,
			})
			require.NoError(t, err)
			defer client.CloseIdleConnections()
			resp, err := client.Get(server.URL)
			require.NoError(t, err)
			defer resp.Body.Close()
			n, err := io.Copy(io.Discard, resp.Body)
			effective := int64(limit)
			if tc.limit > 0 {
				effective = tc.limit.Bytes()
			}
			assert.Equal(t, effective, HTTPClientResponseBodyLimit(client))
			if tc.size > effective {
				assert.ErrorIs(t, err, ErrResponseTooLarge)
				assert.EqualValues(t, effective, n)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.size, n)
			}
		})
	}
}

func TestResponseLimitReader(t *testing.T) {
	readErr := errors.New("read failed")
	for name, tc := range map[string]struct {
		r    io.Reader
		want string
		err  error
	}{
		"empty":             {r: strings.NewReader("")},
		"below":             {r: strings.NewReader("abc"), want: "abc"},
		"exact":             {r: strings.NewReader("abcd"), want: "abcd"},
		"overflow":          {r: strings.NewReader("abcde"), want: "abcd", err: ErrResponseTooLarge},
		"short reads":       {r: iotest.OneByteReader(strings.NewReader("abcde")), want: "abcd", err: ErrResponseTooLarge},
		"data and EOF":      {r: iotest.DataErrReader(strings.NewReader("abcd")), want: "abcd"},
		"source error":      {r: iotest.ErrReader(readErr), err: readErr},
		"error at boundary": {r: io.MultiReader(strings.NewReader("abcd"), iotest.ErrReader(readErr)), want: "abcd", err: readErr},
	} {
		t.Run(name, func(t *testing.T) {
			r := LimitResponseReader(tc.r, 4)
			got, err := io.ReadAll(r)
			assert.Equal(t, tc.want, string(got))
			assert.ErrorIs(t, err, tc.err)
			_, err = r.Read(make([]byte, 1))
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
			} else {
				assert.ErrorIs(t, err, io.EOF)
			}
			n, err := r.Read(nil)
			assert.Zero(t, n)
			assert.NoError(t, err)
		})
	}
}

type observedBody struct {
	io.Reader
	read     int
	closed   bool
	closeErr error
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *observedBody) Close() error { b.closed = true; return b.closeErr }

func TestResponseLimitBodyCleanup(t *testing.T) {
	closeErr := errors.New("close failed")
	source := &observedBody{
		Reader:   repeatingByte('x'),
		closeErr: closeErr,
	}
	body := LimitResponseBody(source, 0)
	assert.Same(t, body, LimitResponseBody(body, 0))
	n, err := body.Read(nil)
	assert.Zero(t, n)
	assert.NoError(t, err)
	assert.Zero(t, source.read)
	_, err = io.Copy(io.Discard, body)
	require.ErrorIs(t, err, ErrResponseTooLarge)
	assert.Equal(t, DefaultResponseBodyLimit+1, source.read)
	CloseBody(&http.Response{
		Body: body,
	})
	assert.True(t, source.closed)
	assert.Equal(t, DefaultResponseBodyLimit+1, source.read)
	assert.ErrorIs(t, body.Close(), closeErr)
}

func TestHTTP2ResponseLimit(t *testing.T) {
	for name, tls := range map[string]bool{"TLS": true, "h2c": false} {
		t.Run(name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.CopyN(w, repeatingByte('x'), DefaultResponseBodyLimit+1)
			})
			var server *httptest.Server
			if tls {
				server = httptest.NewUnstartedServer(handler)
				server.EnableHTTP2 = true
				server.StartTLS()
			} else {
				server = httptest.NewServer(h2c.NewHandler(handler, &http2.Server{}))
			}
			defer server.Close()
			client, err := NewHTTPClient(
				context.Background(),
				ClientConfig{
					ForceHTTP2: true,
					TLSConfig: tlscfg.TLSConfig{
						InsecureSkipVerify: true,
					},
				},
			)
			require.NoError(t, err)
			defer client.CloseIdleConnections()
			resp, err := client.Get(server.URL)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, 2, resp.ProtoMajor)
			n, err := io.Copy(io.Discard, resp.Body)
			assert.ErrorIs(t, err, ErrResponseTooLarge)
			assert.EqualValues(t, DefaultResponseBodyLimit, n)
		})
	}
}

type observedTransport struct {
	body   io.ReadCloser
	closed bool
}

func (t *observedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       t.body,
		Header: http.Header{
			"Content-Encoding": {"gzip"},
		},
		ContentLength: DefaultResponseBodyLimit + 1,
	}, nil
}

func (t *observedTransport) CloseIdleConnections() { t.closed = true }

func TestResponseLimitPreservesRepresentationAndPartialReads(t *testing.T) {
	source := &observedBody{
		Reader: io.MultiReader(strings.NewReader(`{"ok":true}`), repeatingByte(' ')),
	}
	transport := &observedTransport{
		body: source,
	}
	client := NewHTTPClientWithTransport(ClientConfig{}, transport)
	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
	assert.EqualValues(t, DefaultResponseBodyLimit+1, resp.ContentLength)
	assert.False(t, resp.Uncompressed)
	require.NoError(t, resp.Body.Close())
	assert.Zero(t, source.read, "neither RoundTrip nor Close may eagerly read")
	client.CloseIdleConnections()
	assert.True(t, transport.closed)

	// Decode may stop after one object. Cleanup remains bounded; an unread tail
	// does not turn the successfully decoded value into an error.
	source = &observedBody{
		Reader: io.MultiReader(strings.NewReader(`{"ok":true}`), repeatingByte(' ')),
	}
	transport.body = source
	var got map[string]bool
	require.NoError(t, DoHTTP(client).RequestJSON(req, &got))
	assert.Equal(t, map[string]bool{"ok": true}, got)
	assert.Equal(t, DefaultResponseBodyLimit+1, source.read)
}

func BenchmarkHTTPClientResponse(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.CopyN(w, repeatingByte('x'), 64<<10)
	}))
	defer server.Close()
	client, err := NewHTTPClient(context.Background(), ClientConfig{})
	require.NoError(b, err)
	defer client.CloseIdleConnections()
	b.ReportAllocs()
	b.SetBytes(64 << 10)
	b.ResetTimer()
	for b.Loop() {
		resp, err := client.Get(server.URL)
		require.NoError(b, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(b, err)
		require.NoError(b, resp.Body.Close())
	}
}

func TestResponseBodyLimitComposition(t *testing.T) {
	for name, limits := range map[string][2]int64{"same": {4, 4}, "lower": {8, 4}, "higher": {4, 8}} {
		t.Run(name, func(t *testing.T) {
			source := &observedBody{
				Reader: strings.NewReader("123456789"),
			}
			first := LimitResponseBody(source, limits[0])
			second := LimitResponseBody(first, limits[1])
			if limits[0] == limits[1] {
				assert.Same(t, first, second)
			}
			n, err := io.Copy(io.Discard, second)
			require.ErrorIs(t, err, ErrResponseTooLarge)
			assert.EqualValues(t, 4, n)
			var overflow *ResponseBodyLimitError
			require.ErrorAs(t, err, &overflow)
			assert.EqualValues(t, 4, overflow.Limit)
			assert.EqualError(t, err, "HTTP response body exceeds the 4 byte limit")
			require.NoError(t, second.Close())
			assert.True(t, source.closed)
		})
	}
	// Deduplication uses the initial limit even after a partial read.
	body := LimitResponseBody(io.NopCloser(strings.NewReader("12345")), 4)
	_, err := body.Read(make([]byte, 1))
	require.NoError(t, err)
	assert.Same(t, body, LimitResponseBody(body, 4))
	for _, limit := range []int64{0, -1, math.MinInt64} {
		n, err := io.Copy(io.Discard, LimitResponseReader(repeatingByte('x'), limit))
		require.ErrorIs(t, err, ErrResponseTooLarge)
		assert.EqualValues(t, DefaultResponseBodyLimit, n)
		assert.EqualError(t, err, "HTTP response body exceeds the 64 MiB limit")
	}
	n, err := io.Copy(io.Discard, LimitResponseReader(strings.NewReader("abc"), math.MaxInt64))
	assert.NoError(t, err)
	assert.EqualValues(t, 3, n)
}

func TestHTTPClientResponseBodyLimitComposition(t *testing.T) {
	assert.EqualValues(t, DefaultResponseBodyLimit, HTTPClientResponseBodyLimit(nil))
	assert.EqualValues(t, DefaultResponseBodyLimit, HTTPClientResponseBodyLimit(&http.Client{}))
	for _, limits := range [][2]confopt.ByteSize{{4, 8}, {8, 4}, {4, 4}, {128 * confopt.MiB, 256 * confopt.MiB}} {
		source := &observedBody{
			Reader: repeatingByte('x'),
		}
		inner := NewHTTPClientWithTransport(
			ClientConfig{
				ResponseBodyLimit: limits[0],
			},
			&observedTransport{
				body: source,
			},
		)
		outer := NewHTTPClientWithTransport(ClientConfig{
			ResponseBodyLimit: limits[1],
		}, inner.Transport)
		want := min(limits[0], limits[1]).Bytes()
		assert.Equal(t, want, HTTPClientResponseBodyLimit(outer))
		resp, err := outer.Get("http://example.com")
		require.NoError(t, err)
		n, err := io.Copy(io.Discard, resp.Body)
		require.ErrorIs(t, err, ErrResponseTooLarge)
		assert.Equal(t, want, n)
		require.NoError(t, resp.Body.Close())
	}
}

func TestResponseBodyLimitNotUserConfig(t *testing.T) {
	for name, codec := range map[string]struct {
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{"JSON": {json.Marshal, json.Unmarshal}, "YAML": {yaml.Marshal, yaml.Unmarshal}} {
		t.Run(name, func(t *testing.T) {
			cfg := ClientConfig{
				ResponseBodyLimit: 128 * confopt.MiB,
			}
			got, err := codec.marshal(cfg)
			require.NoError(t, err)
			want, err := codec.marshal(ClientConfig{})
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
			data, err := codec.marshal(
				map[string]any{"ResponseBodyLimit": "1MiB", "response_body_limit": "1MiB", "responsebodylimit": 1},
			)
			require.NoError(t, err)
			require.NoError(t, codec.unmarshal(data, &cfg))
			assert.Equal(t, 128*confopt.MiB, cfg.ResponseBodyLimit)
		})
	}
}
