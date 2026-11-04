// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

func TestHTTPResponseBodyLimits(t *testing.T) {
	tests := map[string]struct {
		body string
		gzip bool
		fail bool
	}{
		"exact limit":            {body: `[]` + strings.Repeat(" ", 62)},
		"overflow":               {body: `[]` + strings.Repeat(" ", 63), fail: true},
		"compressed exact limit": {body: `[]` + strings.Repeat(" ", 62), gzip: true},
		"compressed expansion":   {body: `[]` + strings.Repeat(" ", 2048), gzip: true, fail: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			body := []byte(test.body)
			if test.gzip {
				var buffer bytes.Buffer
				writer := gzip.NewWriter(&buffer)
				_, err := writer.Write(body)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				body = buffer.Bytes()
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.gzip {
					w.Header().Set("Content-Encoding", "gzip")
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			httpClient := web.NewHTTPClientWithTransport(web.ClientConfig{
				Timeout:           confopt.Duration(5 * time.Second),
				ResponseBodyLimit: 64,
			}, server.Client().Transport)
			client, err := merakiapi.New(
				merakiapi.Config{
					BaseURL: server.URL + "/api/v1",
					APIKey:  syntheticKey,
				},
				httpClient,
			)
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			if test.fail {
				require.ErrorIs(t, err, web.ErrResponseTooLarge)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []merakiapi.Organization{}, got)
			}
		})
	}
}

type advertisedLimitTransport struct {
	roundTripFunc
	limit int64
}

func (t advertisedLimitTransport) ResponseBodyLimit() int64 { return t.limit }

type trackedBody struct {
	io.Reader
	closed *bool
}

func (b trackedBody) Close() error { *b.closed = true; return nil }

func TestClientEnforcesAdvertisedLimitsAndRecovers(t *testing.T) {
	var calls int
	closed := false
	transport := advertisedLimitTransport{
		limit: 64,
		roundTripFunc: func(r *http.Request) (*http.Response, error) {
			calls++
			body := `[]`
			if calls == 1 {
				body += strings.Repeat(" ", 64)
			}
			resp := response(r, http.StatusOK, nil, "")
			resp.Body = trackedBody{
				Reader: strings.NewReader(body),
				closed: &closed,
			}
			return resp, nil
		},
	}
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: transport,
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	require.ErrorIs(t, err, web.ErrResponseTooLarge)
	assert.Nil(t, got)
	assert.True(t, closed)
	closed = false
	got, err = client.ListOrganizations(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []merakiapi.Organization{}, got)
	assert.True(t, closed)
	assert.Equal(t, 2, calls)
}

func TestLaterPageOverflowReturnsNoInventory(t *testing.T) {
	var calls int
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: advertisedLimitTransport{
			limit: 64,
			roundTripFunc: func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return response(
						r,
						http.StatusOK,
						http.Header{
							"Link": {`<?startingAfter=2>; rel=next`},
						},
						`[{"id":"first"}]`,
					), nil
				}
				return response(r, http.StatusOK, nil, `[]`+strings.Repeat(" ", 64)), nil
			},
		},
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	require.ErrorIs(t, err, web.ErrResponseTooLarge)
	assert.Nil(t, got)
	assert.Equal(t, 2, calls)
}

func TestUnprocessedEncodingIsAnError(t *testing.T) {
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return response(r, http.StatusOK, http.Header{
				"Content-Encoding": {"unknown"},
			}, `[]`), nil
		}),
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	require.ErrorContains(t, err, "unsupported response encoding")
	assert.Nil(t, got)
}
