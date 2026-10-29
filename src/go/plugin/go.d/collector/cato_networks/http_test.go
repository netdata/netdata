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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type repeatingByte byte

func (v repeatingByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(v)
	}
	return len(p), nil
}

func TestCatoResponseLimit(t *testing.T) {
	for name, compressed := range map[string]bool{"plain": false, "explicit gzip": true} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var dst io.Writer = w
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
					gz := gzip.NewWriter(w)
					defer gz.Close()
					dst = gz
				}
				_, _ = io.WriteString(dst, `{"data":{"padding":"`)
				_, _ = io.CopyN(dst, repeatingByte('x'), web.MaxResponseBodySize)
				_, _ = io.WriteString(dst, `"}}`)
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
