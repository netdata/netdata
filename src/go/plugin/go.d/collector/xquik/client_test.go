// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPProfileClient_Profile(t *testing.T) {
	tests := map[string]struct {
		status  int
		body    string
		wantErr string
		check   func(*testing.T, profile)
	}{
		"success": {
			status: http.StatusOK,
			body:   string(dataProfileJSON),
			check: func(t *testing.T, got profile) {
				require.Equal(t, "783214", got.ID)
				require.Equal(t, "netdata", got.Username)
				require.Equal(t, "Netdata", got.Name)
				require.EqualValues(t, 24500, *got.Followers)
				require.EqualValues(t, 175, *got.Following)
				require.EqualValues(t, 8200, *got.StatusesCount)
				require.True(t, *got.Verified)
			},
		},
		"optional fields absent": {
			status: http.StatusOK,
			body:   `{"id":"783214","username":"netdata","name":"Netdata"}`,
			check: func(t *testing.T, got profile) {
				require.Nil(t, got.Followers)
				require.Nil(t, got.Following)
				require.Nil(t, got.StatusesCount)
				require.Nil(t, got.Verified)
			},
		},
		"non-OK response": {
			status:  http.StatusUnauthorized,
			body:    `{"error":"unauthorized"}`,
			wantErr: "HTTP status 401",
		},
		"invalid JSON": {
			status:  http.StatusOK,
			body:    `{`,
			wantErr: "decode Xquik profile response",
		},
		"missing required field": {
			status:  http.StatusOK,
			body:    `{"id":"783214","username":"netdata","name":" "}`,
			wantErr: "required fields id, username, and name must be non-empty",
		},
		"negative count": {
			status:  http.StatusOK,
			body:    `{"id":"783214","username":"netdata","name":"Netdata","followers":-1}`,
			wantErr: "field followers must not be negative",
		},
		"oversized response": {
			status:  http.StatusOK,
			body:    strings.Repeat("x", maxProfileResponseBytes+1),
			wantErr: "body exceeds 1048576 bytes",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/x/users/netdata", r.URL.Path)
				require.Equal(t, "secret", r.Header.Get("x-api-key"))
				require.EqualValues(t, 0, r.ContentLength)
				w.WriteHeader(tc.status)
				_, err := fmt.Fprint(w, tc.body)
				require.NoError(t, err)
			}))
			defer server.Close()

			cfg := New().Config
			cfg.URL = server.URL + "/api/v1"
			cfg.APIKey = "secret"
			client := newProfileClient(cfg, server.Client())

			got, err := client.Profile(context.Background(), "netdata")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestHTTPProfileClient_ProfilePreservesContextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request must not reach the server")
	}))
	defer server.Close()

	cfg := New().Config
	cfg.URL = server.URL + "/api/v1"
	cfg.APIKey = "secret"
	client := newProfileClient(cfg, server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Profile(ctx, "netdata")
	require.ErrorIs(t, err, context.Canceled)
}

func TestCollector_DoesNotFollowRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected = true
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	c := New()
	c.User = "netdata"
	c.APIKey = "secret"
	c.URL = server.URL + "/api/v1"
	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())

	err := c.Check(context.Background())
	require.ErrorContains(t, err, "redirect")
	require.False(t, redirected)
}

func TestHTTPProfileClient_ProfileRequestFailure(t *testing.T) {
	transportErr := errors.New("transport failed")
	cfg := New().Config
	cfg.APIKey = "secret"
	client := newProfileClient(cfg, &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		}),
	})

	_, err := client.Profile(context.Background(), "netdata")
	require.ErrorContains(t, err, "request Xquik profile")
	require.ErrorIs(t, err, transportErr)
}

func TestHTTPProfileClient_ProfileDoesNotDrainOversizedBody(t *testing.T) {
	body := &trackingBody{remaining: maxProfileResponseBytes * 2}
	cfg := New().Config
	cfg.APIKey = "secret"
	client := newProfileClient(cfg, &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
		}),
	})

	_, err := client.Profile(context.Background(), "netdata")
	require.ErrorContains(t, err, "body exceeds 1048576 bytes")
	require.Equal(t, maxProfileResponseBytes+1, body.read)
	require.True(t, body.closed)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type trackingBody struct {
	remaining int
	read      int
	closed    bool
}

func (b *trackingBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	count := min(len(p), b.remaining)
	for i := range count {
		p[i] = 'x'
	}
	b.remaining -= count
	b.read += count
	return count, nil
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}
