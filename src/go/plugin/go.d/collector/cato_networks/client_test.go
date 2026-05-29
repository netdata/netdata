// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

func TestClassifyCatoError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"auth":       {err: errors.New("HTTP 403 forbidden"), want: "auth"},
		"rate limit": {err: errors.New("GraphQL rate limit exceeded"), want: "rate_limit"},
		"timeout":    {err: context.DeadlineExceeded, want: "timeout"},
		"canceled":   {err: fmt.Errorf("wrapped: %w", context.Canceled), want: "canceled"},
		"wrapped timeout": {
			err:  fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			want: "timeout",
		},
		"decode": {
			err:  errors.New("json: cannot unmarshal number into Go struct field"),
			want: "decode",
		},
		"network": {
			err:  &net.DNSError{Err: "no such host", Name: "api.invalid"},
			want: "network",
		},
		"tls": {
			err:  errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			want: "tls",
		},
		"proxy": {
			err:  errors.New("proxyconnect tcp: dial tcp: connection refused"),
			want: "proxy",
		},
		"empty": {
			err:  errors.New("no Cato sites discovered"),
			want: "empty",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, classifyCatoError(tc.err))
		})
	}
}

func TestCatoRequestHeaders(t *testing.T) {
	headers := catoRequestHeaders(map[string]string{
		"Content-Type":    "text/plain",
		"x-api-key":       "wrong-key",
		"X-Account-Id":    "wrong-account",
		"user-agent":      "custom-agent",
		"X-Custom-Header": "custom-value",
	})

	require.False(t, hasCatoHeader(headers, "Content-Type"))
	require.False(t, hasCatoHeader(headers, "x-api-key"))
	require.False(t, hasCatoHeader(headers, "x-account-id"))
	require.True(t, hasCatoHeader(headers, "User-Agent"))
	require.Equal(t, "custom-agent", headers["user-agent"])
	require.Equal(t, "custom-value", headers["X-Custom-Header"])
}

func TestRawGraphQLClient(t *testing.T) {
	tests := map[string]struct {
		handler func(*testing.T, http.ResponseWriter, *http.Request)
		client  func(string, *http.Client) rawGraphQLClient
		check   func(*testing.T, *rawGraphQLClient)
	}{
		"account snapshot uses method account id": {
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "argument-account", r.Header.Get("x-account-id"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Contains(t, string(body), `"accountID":"argument-account"`)
				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write([]byte(`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatusSiteSnapshot":"connected"}]}}}`))
				require.NoError(t, err)
			},
			client: func(url string, httpClient *http.Client) rawGraphQLClient {
				return rawGraphQLClient{url: url, apiKey: "secret", httpClient: httpClient}
			},
			check: func(t *testing.T, client *rawGraphQLClient) {
				snapshot, err := client.AccountSnapshot(context.Background(), "argument-account", []string{"1001"})
				require.NoError(t, err)
				require.Len(t, snapshot.GetAccountSnapshot().GetSites(), 1)
			},
		},
		"does not override reserved headers": {
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				require.Equal(t, "secret", r.Header.Get("x-api-key"))
				require.Equal(t, "argument-account", r.Header.Get("x-account-id"))
				require.Equal(t, "custom-value", r.Header.Get("x-custom-header"))
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"data":{"accountSnapshot":{"sites":[]}}}`))
				require.NoError(t, err)
			},
			client: func(url string, httpClient *http.Client) rawGraphQLClient {
				return rawGraphQLClient{
					url:        url,
					apiKey:     "secret",
					httpClient: httpClient,
					headers: map[string]string{
						"Content-Type":    "text/plain",
						"x-api-key":       "wrong-key",
						"X-Account-Id":    "wrong-account",
						"X-Custom-Header": "custom-value",
					},
				}
			},
			check: func(t *testing.T, client *rawGraphQLClient) {
				_, err := client.AccountSnapshot(context.Background(), "argument-account", nil)
				require.NoError(t, err)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.handler(t, w, r)
			}))
			defer server.Close()

			client := tc.client(server.URL, server.Client())
			tc.check(t, &client)
		})
	}
}

func TestSDKClient(t *testing.T) {
	tests := map[string]struct {
		setup func(*testing.T) (*httptest.Server, Config, *int)
		check func(*testing.T, apiClient, *int)
	}{
		"account snapshot falls back on enum decode error": {
			setup: func(t *testing.T) (*httptest.Server, Config, *int) {
				var calls int
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					_, err := w.Write([]byte(`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatusSiteSnapshot":"degraded","operationalStatusSiteSnapshot":"active"}]}}}`))
					require.NoError(t, err)
				}))
				cfg := Config{AccountID: "12345", APIKey: "secret"}
				cfg.URL = server.URL
				return server, cfg, &calls
			},
			check: func(t *testing.T, client apiClient, calls *int) {
				snapshot, err := client.AccountSnapshot(context.Background(), "12345", []string{"1001"})
				require.NoError(t, err)
				require.Equal(t, 2, *calls)
				require.Len(t, snapshot.GetAccountSnapshot().GetSites(), 1)
				require.Equal(t, "degraded", connectivityStatusString(snapshot.GetAccountSnapshot().GetSites()[0].GetConnectivityStatusSiteSnapshot()))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server, cfg, calls := tc.setup(t)
			defer server.Close()

			client, err := newSDKAPIClient(cfg, server.Client())
			require.NoError(t, err)
			tc.check(t, client, calls)
		})
	}
}

func TestSDKClientClassifiesHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	c := New()
	c.URL = server.URL
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Timeout = confopt.Duration(time.Millisecond)

	require.NoError(t, c.Init(context.Background()))
	err := collectError(t, c)

	require.Error(t, err)
	require.Equal(t, "timeout", classifyCatoError(err))
}
