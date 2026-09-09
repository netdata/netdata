// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

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
				var request rawGraphQLRequest
				require.NoError(t, json.Unmarshal(body, &request))
				require.Contains(t, request.Query, "degradedStatus")
				require.Contains(t, request.Query, "devices {")
				require.Contains(t, request.Query, "socketInfo {")
				require.Contains(t, request.Query, "interfaces {")
				require.Contains(t, request.Query, "interfacesLinkState {")
				require.NotContains(t, request.Query, "users(")
				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(
					[]byte(
						`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":"connected"}]}}}`,
					),
				)
				require.NoError(t, err)
			},
			client: func(url string, httpClient *http.Client) rawGraphQLClient {
				return rawGraphQLClient{
					url:        url,
					apiKey:     "secret",
					httpClient: httpClient,
				}
			},
			check: func(t *testing.T, client *rawGraphQLClient) {
				snapshot, err := client.AccountSnapshot(
					context.Background(),
					"argument-account",
					[]string{"1001"},
				)
				require.NoError(t, err)
				require.Len(t, snapshot.Sites, 1)
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
		"decodes nested snapshot fields": {
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write(
					[]byte(loadTestdata(t, "cato-account-snapshot.schema-shaped.json")),
				)
				require.NoError(t, err)
			},
			client: func(url string, httpClient *http.Client) rawGraphQLClient {
				return rawGraphQLClient{
					url:        url,
					apiKey:     "secret",
					httpClient: httpClient,
				}
			},
			check: func(t *testing.T, client *rawGraphQLClient) {
				snapshot, err := client.AccountSnapshot(
					context.Background(),
					"argument-account",
					[]string{"1001"},
				)
				require.NoError(t, err)
				require.NotEmpty(t, snapshot.Sites)
				require.NotEmpty(t, snapshot.Sites[0].Devices)

				device := snapshot.Sites[0].Devices[0]
				require.Equal(t, "socket-1", derefZero(device.SocketInfo.ID))
				require.Equal(t, "SERIAL-1", derefZero(device.SocketInfo.Serial))
				require.NotEmpty(t, device.Interfaces)
				require.NotNil(t, device.Interfaces[0].Info)
				require.Equal(
					t,
					int64(100000),
					derefZero(device.Interfaces[0].Info.UpstreamBandwidth),
				)
				require.NotEmpty(t, device.InterfacesLinkState)
				require.True(t, derefZero(device.InterfacesLinkState[0].Up))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					tc.handler(t, w, r)
				}),
			)
			defer server.Close()

			client := tc.client(server.URL, server.Client())
			tc.check(t, &client)
		})
	}
}

func TestRawGraphQLClientExplicitGzip(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "gzip", r.Header.Get("Accept-Encoding"))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, err := zw.Write(
				[]byte(
					`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":"connected"}]}}}`,
				),
			)
			require.NoError(t, err)
			require.NoError(t, zw.Close())
		}),
	)
	defer server.Close()

	client := rawGraphQLClient{
		url:        server.URL,
		apiKey:     "secret",
		headers:    map[string]string{"Accept-Encoding": "gzip"},
		httpClient: server.Client(),
	}
	snapshot, err := client.AccountSnapshot(context.Background(), "12345", []string{"1001"})

	require.NoError(t, err)
	require.Len(t, snapshot.Sites, 1)
}

func TestRawGraphQLClientRejectsCorruptGzipTrailer(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(
		[]byte(`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":"connected"}]}}}`),
	)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	body := append([]byte(nil), compressed.Bytes()...)
	body[len(body)-1] ^= 0xff

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "gzip")
			_, err := w.Write(body)
			require.NoError(t, err)
		}),
	)
	defer server.Close()

	client := rawGraphQLClient{
		url:        server.URL,
		apiKey:     "secret",
		headers:    map[string]string{"Accept-Encoding": "gzip"},
		httpClient: server.Client(),
	}
	_, err = client.AccountSnapshot(context.Background(), "12345", []string{"1001"})

	require.ErrorContains(t, err, "json decode:")
}

func TestRawGraphQLClientErrors(t *testing.T) {
	tests := map[string]struct {
		status  int
		body    string
		wantErr string
	}{
		"GraphQL error takes precedence over partial data": {
			body:    `{"data":{"accountSnapshot":{"sites":[]}},"errors":[{"message":"rate limit exceeded"}]}`,
			wantErr: "graphql: rate limit exceeded",
		},
		"network error envelope": {
			body:    `{"networkErrors":[{"message":"upstream unavailable"}]}`,
			wantErr: "graphql: upstream unavailable",
		},
		"client GraphQL error envelope": {
			body:    `{"graphqlErrors":[{"message":"invalid account"}]}`,
			wantErr: "graphql: invalid account",
		},
		"missing data": {
			body:    `{"data":null}`,
			wantErr: "returned no data",
		},
		"missing account snapshot": {
			body:    `{"data":{}}`,
			wantErr: "returned no data",
		},
		"invalid status scalar type": {
			body:    `{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":123}]}}}`,
			wantErr: "cannot unmarshal number",
		},
		"malformed JSON": {
			body:    `{"data":`,
			wantErr: "unexpected EOF",
		},
		"trailing data": {
			body:    `{"data":{"accountSnapshot":{"sites":[]}}} trailing`,
			wantErr: "json decode:",
		},
		"non-success HTTP status": {
			status:  http.StatusBadGateway,
			body:    `upstream unavailable`,
			wantErr: "http status 502",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					status := tc.status
					if status == 0 {
						status = http.StatusOK
					}
					w.WriteHeader(status)
					_, err := w.Write([]byte(tc.body))
					require.NoError(t, err)
				}),
			)
			defer server.Close()

			client := rawGraphQLClient{
				url:        server.URL,
				apiKey:     "secret",
				httpClient: server.Client(),
			}
			_, err := client.AccountSnapshot(context.Background(), "12345", []string{"1001"})
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestRawGraphQLClientPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := rawGraphQLClient{
		url:        "http://127.0.0.1:1",
		apiKey:     "secret",
		httpClient: http.DefaultClient,
	}
	_, err := client.AccountSnapshot(ctx, "12345", []string{"1001"})

	require.ErrorIs(t, err, context.Canceled)
}

func TestSDKClient(t *testing.T) {
	tests := map[string]struct {
		setup func(*testing.T) (*httptest.Server, Config, *int)
		check func(*testing.T, apiClient, *int)
	}{
		"account snapshot tolerates unknown enum representation": {
			setup: func(t *testing.T) (*httptest.Server, Config, *int) {
				var calls int
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						calls++
						w.Header().Set("Content-Type", "application/json")
						_, err := w.Write(
							[]byte(
								`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":"initializing","operationalStatus":"active"}]}}}`,
							),
						)
						require.NoError(t, err)
					}),
				)
				cfg := Config{
					AccountID: "12345",
					APIKey:    "secret",
				}
				cfg.URL = server.URL
				return server, cfg, &calls
			},
			check: func(t *testing.T, client apiClient, calls *int) {
				snapshot, err := client.AccountSnapshot(
					context.Background(),
					"12345",
					[]string{"1001"},
				)
				require.NoError(t, err)
				require.Equal(t, 1, *calls)
				require.Len(t, snapshot.Sites, 1)
				require.Equal(t, "initializing", derefZero(snapshot.Sites[0].ConnectivityStatus))
			},
		},
		"account snapshot reads official degraded status": {
			setup: func(t *testing.T) (*httptest.Server, Config, *int) {
				var calls int
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						calls++
						w.Header().Set("Content-Type", "application/json")
						_, err := w.Write(
							[]byte(
								`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":"connected","degradedStatus":{"isDegraded":true}}]}}}`,
							),
						)
						require.NoError(t, err)
					}),
				)
				cfg := Config{
					AccountID: "12345",
					APIKey:    "secret",
				}
				cfg.URL = server.URL
				return server, cfg, &calls
			},
			check: func(t *testing.T, client apiClient, calls *int) {
				snapshot, err := client.AccountSnapshot(
					context.Background(),
					"12345",
					[]string{"1001"},
				)
				require.NoError(t, err)
				require.Equal(t, 1, *calls)
				require.Len(t, snapshot.Sites, 1)
				require.True(t, snapshot.Sites[0].DegradedStatus.IsDegraded)
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

func BenchmarkSDKClientAccountSnapshot(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(
			[]byte(
				`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatus":"connected","operationalStatus":"active"}]}}}`,
			),
		)
		require.NoError(b, err)
	}))
	b.Cleanup(server.Close)

	cfg := Config{
		AccountID: "12345",
		APIKey:    "secret",
	}
	cfg.URL = server.URL
	client, err := newSDKAPIClient(cfg, server.Client())
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := client.AccountSnapshot(context.Background(), cfg.AccountID, []string{"1001"}); err != nil {
			b.Fatal(err)
		}
	}
}
