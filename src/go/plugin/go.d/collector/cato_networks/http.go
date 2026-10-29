// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// The SDK expands explicit gzip responses internally. Decode and bound them
// before either the SDK or the raw GraphQL parser can allocate the full body.
func newCatoHTTPClient(client *http.Client) *http.Client {
	cloned := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	cloned.Transport = &catoResponseTransport{
		base: base,
	}
	return &cloned
}

type catoResponseTransport struct{ base http.RoundTripper }

func (t *catoResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	resp.Body = web.LimitResponseBody(resp.Body)
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("gzip decode: %w", err)
		}
		resp.Body = web.LimitResponseBody(&gzipResponseBody{
			Reader: gz,
			body:   resp.Body,
		})
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true
	}
	return resp, nil
}

func (t *catoResponseTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type gzipResponseBody struct {
	*gzip.Reader
	body io.Closer
}

func (b *gzipResponseBody) Close() error { return errors.Join(b.Reader.Close(), b.body.Close()) }
