// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

func TestPaginationRejectsUnsafeOrMalformedLinks(t *testing.T) {
	var attackerCalls atomic.Int64
	attacker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attackerCalls.Add(1)
		_, _ = io.WriteString(w, `[]`)
	}))
	defer attacker.Close()
	tests := map[string]string{
		"other origin":         "<" + attacker.URL + `/api/v1/organizations/org/devices?startingAfter=2>; rel=next`,
		"other organization":   `</api/v1/organizations/other/devices?startingAfter=2>; rel=next`,
		"other operation":      `</api/v1/organizations/org/networks?startingAfter=2>; rel=next`,
		"dot path":             `</api/v1/organizations/org/../devices?startingAfter=2>; rel=next`,
		"unexpected filter":    `<?startingAfter=2&statuses[]=online>; rel=next`,
		"time window":          `<?startingAfter=2&t0=2026-10-01T10%3A00%3A00Z>; rel=next`,
		"interval":             `<?startingAfter=2&interval=300>; rel=next`,
		"invalid query":        `<?startingAfter=2;private=SYNTHETIC_PRIVATE_BODY>; rel=next`,
		"duplicate cursor":     `<?startingAfter=2&startingAfter=3>; rel=next`,
		"duplicate page size":  `<?perPage=3&perPage=4>; rel=next`,
		"invalid page size":    `<?perPage=-1>; rel=next`,
		"page below minimum":   `<?perPage=2>; rel=next`,
		"page above maximum":   `<?perPage=5001>; rel=next`,
		"fragment":             `<?startingAfter=2#SYNTHETIC_PRIVATE_BODY>; rel=next`,
		"bad URL":              `<https://%SYNTHETIC_PRIVATE_BODY>; rel=next`,
		"unterminated target":  `<SYNTHETIC_PRIVATE_BODY; rel=next`,
		"unframed target":      `SYNTHETIC_PRIVATE_BODY; rel=next`,
		"unterminated quote":   `<?startingAfter=2>; rel="next`,
		"missing value":        `<?startingAfter=2>; rel`,
		"empty unquoted value": `<?startingAfter=2>; rel=`,
		"ambiguous next":       `<?startingAfter=2>; rel=next, <?startingAfter=3>; rel=next`,
		"trailing separator":   `<?startingAfter=2>; rel=next,`,
		"empty next target":    `<>; rel=next`,
		"bad quoted escape":    `<?startingAfter=2>; title="incomplete\`,
		"bad separator":        `<?startingAfter=2>; rel=next SYNTHETIC_PRIVATE_BODY`,
	}
	for name, link := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Link", link)
				_, _ = io.WriteString(w, `[{"serial":"first"}]`)
			}))
			defer server.Close()
			got, err := clientForServer(t, server).ListDevices(t.Context(), "org")
			require.Error(t, err)
			assert.Nil(t, got)
			assert.EqualValues(t, 1, calls.Load())
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BODY")
		})
	}
	assert.Zero(t, attackerCalls.Load())
}

func TestPaginationDetectsCanonicalCycles(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("startingAfter") == "" {
			w.Header().Set("Link", `<?perPage=9000&startingAfter=%61>; rel=next`)
		} else {
			w.Header().Set("Link", `<?startingAfter=a&perPage=09000>; rel=next`)
		}
		_, _ = io.WriteString(w, `[{"id":"first"}]`)
	}))
	defer server.Close()
	got, err := clientForServer(t, server).ListOrganizations(t.Context())
	require.ErrorContains(t, err, "cyclic pagination")
	assert.Nil(t, got)
	assert.EqualValues(t, 2, calls.Load())
}

func TestPaginationTerminatesOnlyWithoutNext(t *testing.T) {
	for name, header := range map[string][]string{
		"no header":              nil,
		"only previous and last": {`<?endingBefore=2>; rel=prev, <?endingBefore=0>; rel=last`},
		"other relation":         {`<?endingBefore=2>; title="valid, ignored"; rel="prev last"`},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for _, value := range header {
					w.Header().Add("Link", value)
				}
				_, _ = io.WriteString(w, `[]`)
			}))
			defer server.Close()
			got, err := clientForServer(t, server).ListOrganizations(t.Context())
			require.NoError(t, err)
			assert.Empty(t, got)
			assert.NotNil(t, got)
		})
	}
}

func TestPaginationAllowsIdenticalNextRelations(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("startingAfter") == "" {
			for range 2 {
				w.Header().Add("Link", `<?startingAfter=2>; rel=next`)
			}
		}
		_, _ = io.WriteString(w, strings.Repeat(" ", 2)+`[]`)
	}))
	defer server.Close()
	_, err := clientForServer(t, server).ListOrganizations(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 2, calls.Load())
}

func TestLinkExtensionAndRelationSemantics(t *testing.T) {
	tests := map[string]struct {
		link   string
		follow bool
		fail   bool
	}{
		"bare extension":               {link: `<?startingAfter=2>; custom-flag; rel=next`, follow: true},
		"unquoted media type":          {link: `<?startingAfter=2>; type=application/json; rel=next`, follow: true},
		"quoted tab":                   {link: "<?startingAfter=2>; title=\"two\twords\"; rel=next", follow: true},
		"first next relation wins":     {link: `<?startingAfter=2>; rel=next; rel=last`, follow: true},
		"first previous relation wins": {link: `<?startingAfter=2>; rel=prev; rel=next`},
		"unsupported anchor":           {link: `<?startingAfter=2>; anchor="/different-context"; rel=next`, fail: true},
		"empty relation":               {link: `<?startingAfter=2>; rel=""`, fail: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var calls int
			client, err := merakiapi.New(merakiapi.Config{
				APIKey: syntheticKey,
			}, &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return response(r, http.StatusOK, http.Header{
							"Link": {test.link},
						}, `[{"id":"first"}]`), nil
					}
					return response(r, http.StatusOK, nil, `[{"id":"last"}]`), nil
				}),
			})
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			if test.fail {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Equal(t, 1, calls)
			} else {
				require.NoError(t, err)
				want := []merakiapi.Organization{{ID: "first"}}
				if test.follow {
					want = append(want, merakiapi.Organization{
						ID: "last",
					})
				}
				assert.Equal(t, want, got)
				assert.Equal(t, len(want), calls)
			}
		})
	}
}
