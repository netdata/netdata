// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"errors"
	"io"
	"net/http"
)

// MaxResponseBodySize bounds a single HTTP body consumed by the shared clients.
// Explicit decompressors must apply the same bound to their output. This is an
// acquisition limit, not a bound on decoded objects or total process memory.
const MaxResponseBodySize = 64 << 20

// ErrResponseTooLarge identifies reads that exceed MaxResponseBodySize.
var ErrResponseTooLarge = errors.New("HTTP response body exceeds the 64 MiB limit")

// LimitResponseReader caps bytes delivered to a consumer, reporting overflow
// instead of EOF. It reads at most one extra byte to distinguish an exact fit.
func LimitResponseReader(r io.Reader) io.Reader {
	return &responseLimitReader{
		reader:    r,
		remaining: MaxResponseBodySize,
	}
}

// LimitResponseBody bounds reads and forwards Close without draining the body.
func LimitResponseBody(body io.ReadCloser) io.ReadCloser {
	if _, ok := body.(*responseLimitBody); ok {
		return body
	}
	return &responseLimitBody{
		responseLimitReader: responseLimitReader{
			reader:    body,
			remaining: MaxResponseBodySize,
		},
		closer: body,
	}
}

type responseLimitReader struct {
	reader    io.Reader
	remaining int64
	err       error
	probe     [1]byte
}

func (r *responseLimitReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.remaining == 0 {
		n, err := r.reader.Read(r.probe[:])
		if n > 0 {
			err = ErrResponseTooLarge
		}
		r.err = err
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	r.err = err
	return n, err
}

type responseLimitBody struct {
	responseLimitReader
	closer io.Closer
}

func (b *responseLimitBody) Close() error { return b.closer.Close() }

type responseLimitTransport struct{ base http.RoundTripper }

func (t *responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil && resp.Body != nil {
		resp.Body = LimitResponseBody(resp.Body)
	}
	return resp, err
}

func (t *responseLimitTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
