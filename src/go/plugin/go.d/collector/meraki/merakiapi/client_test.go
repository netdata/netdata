// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

const syntheticKey = "SYNTHETIC_API_KEY"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func response(req *http.Request, code int, header http.Header, body string) *http.Response {
	return &http.Response{
		StatusCode:    code,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(body)),
		Request:       req,
		ContentLength: -1,
	}
}

func clientForServer(t *testing.T, server *httptest.Server) *merakiapi.Client {
	t.Helper()
	httpClient := web.NewHTTPClientWithTransport(web.ClientConfig{
		Timeout: confopt.Duration(5 * time.Second),
	}, server.Client().Transport)
	client, err := merakiapi.New(merakiapi.Config{
		BaseURL: server.URL + "/api/v1",
		APIKey:  syntheticKey,
	}, httpClient)
	require.NoError(t, err)
	return client
}

func TestListOperations(t *testing.T) {
	tests := map[string]struct {
		path     string
		perPage  string
		fixture  string
		fetch    func(context.Context, *merakiapi.Client) (any, error)
		expected any
	}{
		"organizations": {
			path: "/api/v1/organizations", perPage: "9000", fixture: "organizations.json",
			fetch:    func(ctx context.Context, c *merakiapi.Client) (any, error) { return c.ListOrganizations(ctx) },
			expected: []merakiapi.Organization{{ID: "12345678901234567890", Name: "Example organization"}, {ID: "2"}},
		},
		"networks": {
			path: "/api/v1/organizations/12345678901234567890/networks", perPage: "1000", fixture: "networks.json",
			fetch: func(ctx context.Context, c *merakiapi.Client) (any, error) {
				return c.ListNetworks(ctx, "12345678901234567890")
			},
			expected: []merakiapi.Network{{
				ID:             "N_EXAMPLE",
				OrganizationID: "12345678901234567890",
				Name:           "Example network",
			}},
		},
		"devices": {
			path: "/api/v1/organizations/12345678901234567890/devices", perPage: "1000", fixture: "devices.json",
			fetch: func(ctx context.Context, c *merakiapi.Client) (any, error) {
				return c.ListDevices(ctx, "12345678901234567890")
			},
			expected: []merakiapi.Device{{
				Serial:      "Q234-ABCD-5678",
				NetworkID:   "N_EXAMPLE",
				Name:        "Example switch",
				Model:       "MS120-8",
				ProductType: "switch",
			}},
		},
		"availabilities": {
			path: "/api/v1/organizations/12345678901234567890/devices/availabilities", perPage: "1000", fixture: "availabilities.json",
			fetch: func(ctx context.Context, c *merakiapi.Client) (any, error) {
				return c.ListDeviceAvailabilities(ctx, "12345678901234567890")
			},
			expected: []merakiapi.DeviceAvailability{
				{
					Serial: "Q234-ABCD-5678",
					Status: "online",
					Network: merakiapi.NetworkRef{
						ID: "N_EXAMPLE",
					},
				},
				{Serial: "Q234-ABCD-5679", Status: "alerting"},
				{Serial: "Q234-ABCD-5680", Status: "offline"},
				{Serial: "Q234-ABCD-5681", Status: "dormant"},
				{Serial: "Q234-ABCD-5682", Status: "future-provider-state"},
				{Serial: "Q234-ABCD-5683"},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile("testdata/" + test.fixture)
			require.NoError(t, err)
			var calls atomic.Int64
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, test.path, r.URL.Path)
				assert.Equal(t, url.Values{
					"perPage": {test.perPage},
				}, r.URL.Query())
				assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				assert.NotEmpty(t, r.UserAgent())
				_, _ = w.Write(body)
			}))
			defer server.Close()
			got, err := test.fetch(t.Context(), clientForServer(t, server))
			require.NoError(t, err)
			assert.Equal(t, test.expected, got)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestPaginationPreservesOpaqueCursorsAndEmptyPages(t *testing.T) {
	var calls atomic.Int64
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
		switch r.URL.Query().Get("startingAfter") {
		case "":
			w.Header().Add("Link", `<?perPage=1000>; rel=first`)
			w.Header().
				Add("Link", `<?perPage=1000&startingAfter=a%2Bb%2Fc%3D>; title="comma, \"quoted\""; rel="prev next"`)
			_, _ = io.WriteString(w, `[{"serial":"first"}]`)
		case "a+b/c=":
			w.Header().
				Set("Link", "<"+server.URL+`/api/v1/organizations/org/devices?perPage=1000&startingAfter=end,token>; rel=next`)
			_, _ = io.WriteString(w, `[]`)
		case "end,token":
			_, _ = io.WriteString(w, `[{"serial":"last"}]`)
		default:
			t.Errorf("unexpected cursor")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	got, err := clientForServer(t, server).ListDevices(t.Context(), "org")
	require.NoError(t, err)
	assert.Equal(t, []merakiapi.Device{{Serial: "first"}, {Serial: "last"}}, got)
	assert.EqualValues(t, 3, calls.Load())
}

func TestLaterPageFailureReturnsNoInventory(t *testing.T) {
	tests := map[string]struct {
		status int
		body   string
	}{
		"HTTP failure": {status: http.StatusForbidden, body: `SYNTHETIC_PRIVATE_BODY`},
		"invalid JSON": {status: http.StatusOK, body: `[{`},
		"null array":   {status: http.StatusOK, body: `null`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Query().Get("startingAfter") == "" {
					w.Header().Set("Link", `<?perPage=9000&startingAfter=2>; rel=next`)
					_, _ = io.WriteString(w, `[{"id":"first"}]`)
					return
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			got, err := clientForServer(t, server).ListOrganizations(t.Context())
			require.Error(t, err)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BODY")
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestInvalidArrayResponses(t *testing.T) {
	tests := map[string]string{
		"null": "null", "object envelope": `{"items":[]}`, "null item": `[null]`,
		"wrong item": `[1]`, "trailing JSON": `[] {"private":"SYNTHETIC_PRIVATE_BODY"}`,
		"trailing garbage": `[] SYNTHETIC_PRIVATE_BODY`, "invalid JSON": `SYNTHETIC_PRIVATE_BODY`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			got, err := clientForServer(t, server).ListOrganizations(t.Context())
			require.Error(t, err)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BODY")
		})
	}
}

func TestEmptyAndOptionalFields(t *testing.T) {
	for name, body := range map[string]string{"empty": "[] \n\t", "optional fields": "[{}]"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			got, err := clientForServer(t, server).ListOrganizations(t.Context())
			require.NoError(t, err)
			if name == "empty" {
				assert.Equal(t, []merakiapi.Organization{}, got)
			} else {
				assert.Equal(t, []merakiapi.Organization{{}}, got)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := clientForServer(t, server)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		got, err := client.ListOrganizations(ctx)
		assert.Nil(t, got)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach HTTP server")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("request did not honor cancellation")
	}
}

func TestCanceledContextAndInvalidOrganizationMakeNoRequests(t *testing.T) {
	var calls atomic.Int64
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return response(r, http.StatusOK, nil, `[]`), nil
		}),
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := client.ListOrganizations(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
	expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	got, err = client.ListOrganizations(expired)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, got)
	for _, org := range []string{"", ".", ".."} {
		items, err := client.ListDevices(t.Context(), org)
		require.Error(t, err)
		assert.Nil(t, items)
	}
	assert.Zero(t, calls.Load())
}

func TestOrganizationPathEscaping(t *testing.T) {
	var gotPath string
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotPath = r.URL.EscapedPath()
			return response(r, http.StatusOK, nil, `[]`), nil
		}),
	})
	require.NoError(t, err)
	_, err = client.ListDevices(t.Context(), "opaque /?%")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/organizations/opaque%20%2F%3F%25/devices", gotPath)
}

func TestConcurrentLists(t *testing.T) {
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
			return response(r, http.StatusOK, nil, `[{"id":"1"}]`), nil
		}),
	})
	require.NoError(t, err)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			got, err := client.ListOrganizations(t.Context())
			assert.NoError(t, err)
			assert.Equal(t, []merakiapi.Organization{{ID: "1"}}, got)
		})
	}
	group.Wait()
}

func TestNewRejectsInvalidConstructionWithoutEchoingInput(t *testing.T) {
	tests := map[string]merakiapi.Config{
		"plaintext":          {BaseURL: "http://example.com/api/v1", APIKey: syntheticKey},
		"credentials in URL": {BaseURL: "https://user:SYNTHETIC_PRIVATE_BODY@example.com/api/v1", APIKey: syntheticKey},
		"query": {
			BaseURL: "https://example.com/api/v1?private=SYNTHETIC_PRIVATE_BODY",
			APIKey:  syntheticKey,
		},
		"fragment":         {BaseURL: "https://example.com/api/v1#SYNTHETIC_PRIVATE_BODY", APIKey: syntheticKey},
		"bad escape":       {BaseURL: "https://example.com/%SYNTHETIC_PRIVATE_BODY", APIKey: syntheticKey},
		"wrong path":       {BaseURL: "https://example.com/v0", APIKey: syntheticKey},
		"missing auth":     {},
		"two credentials":  {APIKey: syntheticKey, BearerTokenFile: "SYNTHETIC_PRIVATE_BODY"},
		"header injection": {APIKey: "SYNTHETIC_PRIVATE_BODY\r\ninjected"},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := merakiapi.New(cfg, &http.Client{})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BODY")
			assert.NotContains(t, err.Error(), syntheticKey)
		})
	}
	_, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, nil)
	require.Error(t, err)
}

func TestTransportErrorKeepsCauseWithoutURL(t *testing.T) {
	for name, cause := range map[string]error{
		"connection failure": errors.New("dial tcp 192.0.2.1:443: connect: connection refused"),
		"wrapped deadline":   fmt.Errorf("i/o timeout: %w", context.DeadlineExceeded),
	} {
		t.Run(name, func(t *testing.T) {
			client, err := merakiapi.New(merakiapi.Config{
				APIKey: syntheticKey,
			}, &http.Client{
				Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause }),
			})
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			assert.Nil(t, got)
			require.ErrorIs(t, err, cause)
			// The exact text proves the url.Error wrapper and its request URL are gone.
			assert.Equal(t, "getOrganizations: HTTP request failed: "+cause.Error(), err.Error())
		})
	}
}
