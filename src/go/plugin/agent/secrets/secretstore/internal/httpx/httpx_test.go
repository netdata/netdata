// SPDX-License-Identifier: GPL-3.0-or-later

package httpx

import (
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInsecureClient_PreservesDefaultTransportBehavior(t *testing.T) {
	client := VaultInsecureClient(5 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, defaultTransport.Proxy)

	assert.Equal(t, 5*time.Second, client.Timeout)
	require.NotNil(t, client.CheckRedirect)

	assert.NotSame(t, defaultTransport, transport)
	assert.NotNil(t, transport.Proxy)
	assert.Equal(t, defaultTransport.MaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, defaultTransport.IdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, defaultTransport.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)

	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestClientsDoNotFollowTemporaryRedirects(t *testing.T) {
	tests := map[string]struct {
		client func(time.Duration) *http.Client
	}{
		"Vault client": {
			client: VaultClient,
		},
		"no-proxy client": {
			client: NoProxyClient,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tokenSeen := make(chan string, 1)
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenSeen <- r.Header.Get("X-Test-Token")
				w.WriteHeader(http.StatusOK)
			}))
			defer target.Close()

			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			}))
			defer redirect.Close()

			req, err := http.NewRequest(http.MethodGet, redirect.URL, nil)
			require.NoError(t, err)
			req.Header.Set("X-Test-Token", "test-token")

			resp, err := tc.client(5 * time.Second).Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			select {
			case seen := <-tokenSeen:
				t.Fatalf("unexpected redirected request carrying token %q", seen)
			default:
			}
		})
	}
}

func TestNoProxyClient_PreservesDefaultTransportBehavior(t *testing.T) {
	client := NoProxyClient(2 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)

	assert.Equal(t, 2*time.Second, client.Timeout)
	assert.NotSame(t, defaultTransport, transport)
	assert.Nil(t, transport.Proxy)
	assert.Equal(t, defaultTransport.MaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, defaultTransport.IdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, defaultTransport.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	assert.NotNil(t, client.CheckRedirect)
}

func TestReadResponseBody(t *testing.T) {
	readErr := errors.New("read failed")
	tests := map[string]struct {
		reader  io.Reader
		limit   int64
		want    string
		wantErr error
	}{
		"empty body": {
			reader: strings.NewReader(""),
			limit:  4,
		},
		"body below limit": {
			reader: strings.NewReader("abc"),
			limit:  4,
			want:   "abc",
		},
		"body at limit": {
			reader: strings.NewReader("abcd"),
			limit:  4,
			want:   "abcd",
		},
		"body above limit": {
			reader:  strings.NewReader("abcde"),
			limit:   4,
			want:    "abcd",
			wantErr: ErrResponseTooLarge,
		},
		"read failure": {
			reader:  failingReader{err: readErr},
			limit:   4,
			wantErr: readErr,
		},
		"negative limit": {
			reader:  strings.NewReader("abc"),
			limit:   -1,
			wantErr: errors.New("HTTP response size limit cannot be negative"),
		},
		"maximum integer limit": {
			reader:  strings.NewReader("abc"),
			limit:   math.MaxInt64,
			wantErr: errors.New("HTTP response size limit is too large"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			body, err := ReadResponseBody(tc.reader, tc.limit)

			assert.Equal(t, tc.want, string(body))
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			if errors.Is(tc.wantErr, ErrResponseTooLarge) || errors.Is(tc.wantErr, readErr) {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.EqualError(t, err, tc.wantErr.Error())
		})
	}
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }
