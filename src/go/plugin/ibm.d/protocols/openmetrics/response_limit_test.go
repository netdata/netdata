// SPDX-License-Identifier: GPL-3.0-or-later

package openmetrics

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
				_, _ = io.CopyN(dst, repeatingByte('x'), web.MaxResponseBodySize)
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
