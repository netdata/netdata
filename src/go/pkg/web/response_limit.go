// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// DefaultResponseBodyLimit bounds a single HTTP body consumed by the shared clients
// unless the caller supplies a positive ResponseBodyLimit in ClientConfig.
// Explicit decompressors must apply the same bound to their output. This is an
// acquisition limit, not a bound on decoded objects or total process memory.
const DefaultResponseBodyLimit = 64 << 20

// ErrResponseTooLarge identifies reads that exceed the response-body limit.
var ErrResponseTooLarge = errors.New("HTTP response body exceeds size limit")

// ResponseBodyLimitError reports the byte limit exceeded by a response.
type ResponseBodyLimitError struct {
	Limit int64
}

func (e *ResponseBodyLimitError) Error() string {
	if e.Limit%(1<<20) == 0 {
		return fmt.Sprintf("HTTP response body exceeds the %d MiB limit", e.Limit>>20)
	}
	return fmt.Sprintf("HTTP response body exceeds the %d byte limit", e.Limit)
}

func (e *ResponseBodyLimitError) Unwrap() error { return ErrResponseTooLarge }

func responseBodyLimit(limit int64) int64 {
	if limit <= 0 {
		return DefaultResponseBodyLimit
	}
	return limit
}

// HTTPClientResponseBodyLimit returns the client's response policy, or the default
// for an unknown client. Transport adapters must forward ResponseBodyLimit() int64
// to preserve a wrapped client's policy. The method describes a byte limit, not
// whether the transport enforces it; acquisition helpers still bound their reads.
func HTTPClientResponseBodyLimit(client *http.Client) int64 {
	if client != nil {
		if policy, ok := client.Transport.(interface{ ResponseBodyLimit() int64 }); ok {
			return responseBodyLimit(policy.ResponseBodyLimit())
		}
	}
	return DefaultResponseBodyLimit
}

// LimitResponseReader caps bytes delivered to a consumer, reporting overflow
// instead of EOF. It reads at most one extra byte to distinguish an exact fit.
// Nonpositive limits select DefaultResponseBodyLimit.
func LimitResponseReader(r io.Reader, limit int64) io.Reader {
	limit = responseBodyLimit(limit)
	return &responseLimitReader{
		reader:    r,
		limit:     limit,
		remaining: limit,
	}
}

// LimitResponseBody bounds reads and forwards Close without draining the body.
// Nonpositive limits select DefaultResponseBodyLimit. Existing limits remain in
// force; wrapping a body cannot raise a limit already imposed on that body.
func LimitResponseBody(body io.ReadCloser, limit int64) io.ReadCloser {
	limit = responseBodyLimit(limit)
	if bounded, ok := body.(*responseLimitBody); ok && bounded.limit == limit {
		return body
	}
	return &responseLimitBody{
		responseLimitReader: responseLimitReader{
			reader:    body,
			limit:     limit,
			remaining: limit,
		},
		closer: body,
	}
}

type responseLimitReader struct {
	reader    io.Reader
	limit     int64
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
			err = &ResponseBodyLimitError{
				Limit: r.limit,
			}
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

type responseLimitTransport struct {
	base  http.RoundTripper
	limit int64
}

func (t *responseLimitTransport) ResponseBodyLimit() int64 { return t.limit }

func (t *responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil && resp.Body != nil {
		resp.Body = LimitResponseBody(resp.Body, t.limit)
	}
	return resp, err
}

func (t *responseLimitTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
