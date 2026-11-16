// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

func TestAuthenticatedRegionalRedirectAndPagination(t *testing.T) {
	for _, root := range []string{"meraki.com", "meraki.ca", "meraki.cn", "meraki.in", "gov-meraki.com"} {
		t.Run(root, func(t *testing.T) {
			var hosts []string
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				hosts = append(hosts, r.URL.Hostname())
				assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				switch r.URL.Hostname() {
				case "api." + root:
					location := *r.URL
					location.Host = "n23." + root
					return response(r, http.StatusFound, http.Header{
						"Location": {location.String()},
					}, ""), nil
				case "n23." + root:
					location := *r.URL
					location.Host = "n23.dashboard." + root
					return response(
						r,
						http.StatusTemporaryRedirect,
						http.Header{
							"Location": {location.String()},
						},
						"",
					), nil
				case "n23.dashboard." + root:
					if r.URL.Query().Get("startingAfter") == "" {
						return response(r, http.StatusOK, http.Header{
							"Link": {`<?perPage=1000&startingAfter=last>; rel=next`},
						}, `[{"serial":"first"}]`), nil
					}
					return response(r, http.StatusOK, nil, `[{"serial":"last"}]`), nil
				default:
					t.Errorf("unexpected credential destination")
					return response(r, http.StatusForbidden, nil, ""), nil
				}
			})
			var originalRedirectCalls int
			httpClient := &http.Client{
				Transport: transport,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					originalRedirectCalls++
					return errors.New("original policy")
				},
			}
			client, err := merakiapi.New(merakiapi.Config{
				BaseURL: "https://api." + root + "/api/v1",
				APIKey:  syntheticKey,
			}, httpClient)
			require.NoError(t, err)
			got, err := client.ListDevices(t.Context(), "org")
			require.NoError(t, err)
			assert.Equal(t, []merakiapi.Device{{Serial: "first"}, {Serial: "last"}}, got)
			assert.Equal(
				t,
				[]string{"api." + root, "n23." + root, "n23.dashboard." + root, "n23.dashboard." + root},
				hosts,
			)
			assert.Zero(t, originalRedirectCalls)
			assert.Error(t, httpClient.CheckRedirect(nil, nil))
			assert.Equal(t, 1, originalRedirectCalls)
			// A separate operation starts at the configured API, without caching a shard.
			_, err = client.ListDevices(t.Context(), "org")
			require.NoError(t, err)
			assert.Equal(t, "api."+root, hosts[4])
		})
	}
}

func TestRedirectRejectsUntrustedTargetsBeforeSendingCredentials(t *testing.T) {
	const operation = "/api/v1/organizations?perPage=9000"
	tests := map[string]string{
		"other domain":           "https://example.com" + operation,
		"subdomain suffix trick": "https://n23.dashboard.meraki.com.example.com" + operation,
		"unknown subdomain":      "https://unverified.meraki.com" + operation,
		"cross region":           "https://api.meraki.ca" + operation,
		"root domain":            "https://meraki.com" + operation,
		"downgrade":              "http://n23.meraki.com" + operation,
		"other port":             "https://n23.meraki.com:444" + operation,
		"userinfo":               "https://user:SYNTHETIC_PRIVATE_BODY@n23.meraki.com" + operation,
		"fragment":               "https://n23.meraki.com" + operation + "#SYNTHETIC_PRIVATE_BODY",
		"different operation":    "https://n23.meraki.com/api/v1/organizations/other/devices?perPage=9000",
		"different query":        "https://n23.meraki.com/api/v1/organizations?perPage=9000&startingAfter=2",
		"malformed query":        "https://n23.meraki.com/api/v1/organizations?perPage=9000;private=SYNTHETIC_PRIVATE_BODY",
	}
	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			var calls int
			client, err := merakiapi.New(merakiapi.Config{
				APIKey: syntheticKey,
			}, &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					assert.Equal(t, "api.meraki.com", r.URL.Hostname())
					return response(r, http.StatusFound, http.Header{
						"Location": {target},
					}, ""), nil
				}),
			})
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			require.ErrorContains(t, err, "Meraki redirect")
			assert.Nil(t, got)
			assert.Equal(t, 1, calls)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BODY")
			assert.NotContains(t, err.Error(), target)
		})
	}
}

func TestUnparsableRedirectLocationIsReported(t *testing.T) {
	var calls int
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return response(r, http.StatusFound, http.Header{
				"Location": {"https://redirect-secret.invalid/%zz?token=synthetic-redirect-secret"},
			}, ""), nil
		}),
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	// net/http rejects the header before any redirect request; its reason is shown.
	require.ErrorContains(t, err, "getOrganizations: HTTP request failed: failed to parse Location header")
	assert.NotContains(t, err.Error(), "redirect-secret")
	assert.Nil(t, got)
	assert.Equal(t, 1, calls)
}

func TestRedirectHopLimit(t *testing.T) {
	var calls int
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			location := *r.URL
			location.Host = fmt.Sprintf("n%d.meraki.com", calls)
			return response(r, http.StatusFound, http.Header{
				"Location": {location.String()},
			}, ""), nil
		}),
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, 10, calls)
}

func TestPaginationAlsoValidatesRegionalHosts(t *testing.T) {
	for name, host := range map[string]string{
		"trusted": "n2.dashboard.meraki.com", "wrong region": "n2.dashboard.meraki.cn",
		"lookalike": "n2.dashboard.meraki.com.example.com", "unknown": "data.meraki.com",
	} {
		t.Run(name, func(t *testing.T) {
			var calls int
			client, err := merakiapi.New(merakiapi.Config{
				APIKey: syntheticKey,
			}, &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return response(r, http.StatusOK, http.Header{
							"Link": {"<https://" + host + `/api/v1/organizations?startingAfter=2>; rel=next`},
						}, `[{"id":"first"}]`), nil
					}
					assert.Equal(t, host, r.URL.Hostname())
					assert.Equal(t, "Bearer "+syntheticKey, r.Header.Get("Authorization"))
					return response(r, http.StatusOK, nil, `[{"id":"last"}]`), nil
				}),
			})
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			if name == "trusted" {
				require.NoError(t, err)
				assert.Equal(t, []merakiapi.Organization{{ID: "first"}, {ID: "last"}}, got)
				assert.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Equal(t, 1, calls)
			}
		})
	}
}

func TestRedirectAllowsEquivalentQueryEncoding(t *testing.T) {
	var calls int
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: syntheticKey,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				location := *r.URL
				location.RawQuery = "perPage=%39%30%30%30"
				return response(r, http.StatusFound, http.Header{
					"Location": {location.String()},
				}, ""), nil
			}
			assert.Equal(t, url.Values{
				"perPage": {"9000"},
			}, r.URL.Query())
			assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "))
			return response(r, http.StatusOK, nil, `[]`), nil
		}),
	})
	require.NoError(t, err)
	_, err = client.ListOrganizations(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}
